#include "keystar/client.hpp"
#include "keystar/json_parser.hpp"

#include <sstream>

namespace keystar {

Client::Client(ClientOptions options)
    : options_(std::move(options)) {
    // Production defaults are created in the constructor body via
    // factory functions (defined in the platform-specific translation
    // units). For now, the caller must supply them via the DI ctor.
}

Client::Client(ClientOptions options,
               std::shared_ptr<Transport> transport,
               std::shared_ptr<DeviceIdentityProvider> deviceProvider,
               std::shared_ptr<TokenStore> tokenStore)
    : options_(std::move(options)),
      transport_(std::move(transport)),
      deviceProvider_(std::move(deviceProvider)),
      tokenStore_(std::move(tokenStore)) {
    // Restore any previously saved session.
    if (auto saved = tokenStore_->load()) {
        refreshToken_ = saved->refresh_token;
        userId_ = saved->user_id;
        deviceId_ = saved->device_id;
        licenseId_ = saved->license_id;
    }
}

std::string Client::buildUrl(const std::string& path) const {
    std::string url = options_.base_url;
    if (!url.empty() && url.back() != '/') url += '/';
    if (!path.empty() && path.front() == '/') url += path.substr(1);
    else url += path;
    return url;
}

HttpRequest Client::makeRequest(HttpMethod method, const std::string& path,
                                const std::string& body) const {
    HttpRequest req;
    req.method = method;
    req.url = buildUrl(path);
    req.headers["Content-Type"] = "application/json";
    req.headers["X-KeyStar-App"] = options_.application_id;
    if (!accessToken_.empty()) {
        req.headers["Authorization"] = "Bearer " + accessToken_;
    } else {
        req.headers["Authorization"] = "Bearer " + options_.publishable_key;
    }
    req.body = body;
    return req;
}

ApiResponse<SessionResult> Client::login(const std::string& email,
                                         const std::string& password) {
    std::lock_guard lock(mutex_);

    // Step 1: Collect device identity.
    DeviceIdentity identity;
    DeviceProof proof;
    if (deviceProvider_) {
        identity = deviceProvider_->collect();
    }

    // Step 2: POST /v1/auth/login
    {
        std::ostringstream oss;
        oss << "{\"email\":" << JsonValue::parse("\"" + email + "\"").stringValue()
            << ",\"password\":" << JsonValue::parse("\"" + password + "\"").stringValue()
            << ",\"device_fingerprint\":\"" << identity.fingerprint << "\"}";

        auto resp = transport_->send(makeRequest(HttpMethod::Post, "/v1/auth/login", oss.str()));
        if (!resp.ok()) {
            auto json = JsonValue::parse(resp.body);
            return ApiResponse<SessionResult>{
                .error_code = json.getString("code"),
                .error_message = json.getString("message"),
            };
        }

        auto json = JsonValue::parse(resp.body);
        if (!json.getBool("ok")) {
            return ApiResponse<SessionResult>{
                .error_code = json.getString("code"),
                .error_message = json.getString("message"),
            };
        }

        // Step 3: Sign challenge.
        std::string challengeB64 = json.getString("challenge");
        std::string sessionId = json.getString("session_id");

        // Decode the base64 challenge to raw bytes.
        // (Simplified: in production, use a proper base64 decoder.)
        proof = deviceProvider_->signChallenge(
            std::span<const uint8_t>(
                reinterpret_cast<const uint8_t*>(challengeB64.data()),
                challengeB64.size()));

        // Step 4: POST /v1/device/verify
        {
            std::ostringstream voss;
            voss << "{\"session_id\":\"" << sessionId
                 << "\",\"challenge\":\"" << challengeB64
                 << "\",\"challenge_signature\":\"" << proof.challenge_signature
                 << "\",\"tpm_public_key\":\"" << proof.device_public_key
                 << "\",\"hardware\":{\"smbios_uuid\":\"" << identity.smbios_uuid
                 << "\",\"motherboard_serial\":\"" << identity.motherboard_serial
                 << "\",\"bios_serial\":\"" << identity.bios_serial
                 << "\",\"system_disk_serial\":\"" << identity.system_disk_serial
                 << "\",\"machine_guid\":\"" << identity.machine_guid
                 << "\",\"fingerprint\":\"" << identity.fingerprint << "\"}}";

            auto vresp = transport_->send(makeRequest(HttpMethod::Post, "/v1/device/verify", voss.str()));
            if (!vresp.ok()) {
                auto vjson = JsonValue::parse(vresp.body);
                return ApiResponse<SessionResult>{
                    .error_code = vjson.getString("code"),
                    .error_message = vjson.getString("message"),
                };
            }

            auto vjson = JsonValue::parse(vresp.body);
            if (!vjson.getBool("ok")) {
                return ApiResponse<SessionResult>{
                    .error_code = vjson.getString("code"),
                    .error_message = vjson.getString("message"),
                };
            }

            // Step 5: Store session.
            SessionResult session;
            session.access_token = vjson.getString("token");
            session.refresh_token = vjson.getString("refresh_token");
            session.token_expires_at = vjson.getString("token_expires_at");
            session.license_id = vjson.getString("license_id");
            session.device_id = vjson.getString("device_id");

            storeSession(session);
            return ApiResponse<SessionResult>{.ok = true, .data = std::move(session)};
        }
    }
}

ApiResponse<SessionResult> Client::refresh() {
    std::lock_guard lock(mutex_);

    if (refreshToken_.empty()) {
        return ApiResponse<SessionResult>{
            .error_code = "INVALID_REFRESH_TOKEN",
            .error_message = "no refresh token available",
        };
    }

    std::ostringstream oss;
    oss << "{\"refresh_token\":\"" << refreshToken_ << "\"}";

    auto resp = transport_->send(makeRequest(HttpMethod::Post, "/v1/auth/refresh", oss.str()));
    auto json = JsonValue::parse(resp.body);

    if (!resp.ok() || !json.getBool("ok")) {
        clearSession();
        return ApiResponse<SessionResult>{
            .error_code = json.getString("code"),
            .error_message = json.getString("message"),
        };
    }

    SessionResult session;
    session.access_token = json.getString("access_token");
    session.refresh_token = json.getString("refresh_token");
    session.token_expires_at = json.getString("token_expires_at");
    session.license_id = licenseId_;
    session.device_id = deviceId_;

    storeSession(session);
    return ApiResponse<SessionResult>{.ok = true, .data = std::move(session)};
}

ApiResponse<UserProfile> Client::me() {
    std::lock_guard lock(mutex_);

    auto resp = transport_->send(makeRequest(HttpMethod::Get, "/v1/me"));
    auto json = JsonValue::parse(resp.body);

    if (!resp.ok() || !json.getBool("ok")) {
        return ApiResponse<UserProfile>{
            .error_code = json.getString("code"),
            .error_message = json.getString("message"),
        };
    }

    UserProfile user;
    user.id = json.getString("user.id");
    user.email = json.getString("user.email");
    user.status = json.getString("user.status");
    user.created_at = json.getString("user.created_at");

    return ApiResponse<UserProfile>{.ok = true, .data = std::move(user)};
}

bool Client::logout() {
    std::lock_guard lock(mutex_);
    if (!refreshToken_.empty()) {
        std::ostringstream oss;
        oss << "{\"refresh_token\":\"" << refreshToken_ << "\"}";
        transport_->send(makeRequest(HttpMethod::Post, "/v1/auth/logout", oss.str()));
    }
    clearSession();
    return true;
}

bool Client::isAuthenticated() const noexcept {
    std::lock_guard lock(mutex_);
    return !accessToken_.empty() || !refreshToken_.empty();
}

std::string Client::accessToken() const {
    std::lock_guard lock(mutex_);
    return accessToken_;
}

void Client::storeSession(const SessionResult& session) {
    accessToken_ = session.access_token;
    refreshToken_ = session.refresh_token;
    tokenExpiresAt_ = session.token_expires_at;
    userId_ = session.user.id;
    deviceId_ = session.device_id;
    licenseId_ = session.license_id;

    if (tokenStore_ && !refreshToken_.empty()) {
        StoredSession stored;
        stored.refresh_token = refreshToken_;
        stored.user_id = userId_;
        stored.device_id = deviceId_;
        stored.license_id = licenseId_;
        stored.expires_at = tokenExpiresAt_;
        tokenStore_->save(stored);
    }
}

void Client::clearSession() {
    accessToken_.clear();
    refreshToken_.clear();
    tokenExpiresAt_.clear();
    userId_.clear();
    deviceId_.clear();
    licenseId_.clear();
    if (tokenStore_) {
        tokenStore_->clear();
    }
}

}  // namespace keystar
