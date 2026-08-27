#include "keystar/client.hpp"
#include "keystar/json_parser.hpp"
#include "keystar/transport.hpp"
#include "keystar/device_identity.hpp"
#include "keystar/token_store.hpp"

#include <cassert>
#include <cstdio>
#include <memory>
#include <string>
#include <stdexcept>
#include <vector>

namespace {

/// FakeTransport records requests and returns pre-configured responses.
class FakeTransport : public keystar::Transport {
public:
    struct RecordedRequest {
        keystar::HttpMethod method;
        std::string url;
		std::string body;
		std::map<std::string, std::string> headers;
    };

    std::vector<RecordedRequest> requests;
    std::vector<keystar::HttpResponse> responses;

    keystar::HttpResponse send(const keystar::HttpRequest& request) override {
		requests.push_back({request.method, request.url, request.body, request.headers});
        if (!responses.empty()) {
            auto resp = responses.front();
            responses.erase(responses.begin());
            return resp;
        }
        return {.status_code = 200, .body = "{\"ok\":true}"};
    }
};

/// FakeDeviceProvider returns deterministic identity and signature.
class FakeDeviceProvider : public keystar::DeviceIdentityProvider {
public:
    keystar::DeviceIdentity collect() override {
        return {
            .smbios_uuid = "fake-smbios-uuid",
            .motherboard_serial = "fake-mobo-serial",
            .bios_serial = "fake-bios-serial",
            .system_disk_serial = "fake-disk-serial",
            .machine_guid = "fake-guid",
            .fingerprint = "fake-fingerprint",
            .tpm_public_key = "fake-tpm-pubkey",
            .tpm_public_key_sha256 = "fake-sha256",
        };
    }

    keystar::DeviceProof signChallenge(std::span<const uint8_t>) override {
        return {
            .challenge_signature = "fake-signature",
            .device_public_key = "fake-tpm-pubkey",
        };
    }

    bool hasTpm() const noexcept override { return true; }
};

/// FakeTokenStore records save/load/clear calls.
class FakeTokenStore : public keystar::TokenStore {
public:
    bool save(const keystar::StoredSession& s) override {
        savedSession = s;
        return true;
    }

    std::optional<keystar::StoredSession> load() override {
        return savedSession;
    }

    bool clear() override {
        savedSession = std::nullopt;
        return true;
    }

    std::optional<keystar::StoredSession> savedSession;
};

void testLoginSuccess() {
    auto transport = std::make_shared<FakeTransport>();
    auto deviceProvider = std::make_shared<FakeDeviceProvider>();
    auto tokenStore = std::make_shared<FakeTokenStore>();

    // Login response
    transport->responses.push_back({
        .status_code = 200,
        .body = R"({"ok":true,"session_id":"sess-123","challenge":"Y2hhbGxlbmdl"})"
    });
    // Verify response
    transport->responses.push_back({
        .status_code = 200,
        .body = R"({"ok":true,"token":"access-token-abc","refresh_token":"refresh-token-xyz","token_expires_at":"2026-09-20T00:00:00Z","license_id":"lic-1","device_id":"dev-1"})"
    });

    keystar::Client client(
        {.application_id = "app-1", .publishable_key = "ks_pk_live_..."},
        transport, deviceProvider, tokenStore
    );

    auto result = client.login("test@example.com", "password123");
    assert(result.ok);
    assert(result.data.access_token == "access-token-abc");
    assert(result.data.refresh_token == "refresh-token-xyz");
    assert(result.data.license_id == "lic-1");
    assert(result.data.device_id == "dev-1");

    // Verify 2 requests were made (login + verify).
    assert(transport->requests.size() == 2);
    assert(transport->requests[0].url.find("/v1/auth/login") != std::string::npos);
    assert(transport->requests[1].url.find("/v1/device/verify") != std::string::npos);

    // Verify token was stored.
    assert(tokenStore->savedSession.has_value());
    assert(tokenStore->savedSession->refresh_token == "refresh-token-xyz");

    printf("  PASS testLoginSuccess\n");
}

void testLoginFailure() {
    auto transport = std::make_shared<FakeTransport>();
    auto deviceProvider = std::make_shared<FakeDeviceProvider>();
    auto tokenStore = std::make_shared<FakeTokenStore>();

    transport->responses.push_back({
        .status_code = 401,
        .body = R"({"ok":false,"code":"INVALID_CREDENTIALS","message":"invalid credentials"})"
    });

    keystar::Client client(
        {.application_id = "app-1", .publishable_key = "ks_pk_live_..."},
        transport, deviceProvider, tokenStore
    );

    auto result = client.login("bad@example.com", "wrong");
    assert(!result.ok);
    assert(result.error_code == "INVALID_CREDENTIALS");

    // Only 1 request (login failed, no verify).
    assert(transport->requests.size() == 1);

    printf("  PASS testLoginFailure\n");
}

void testClientRejectsUnsafeBaseUrlAndEscapesJson() {
	auto transport = std::make_shared<FakeTransport>();
	auto deviceProvider = std::make_shared<FakeDeviceProvider>();
	auto tokenStore = std::make_shared<FakeTokenStore>();
	bool rejected = false;
	try {
		keystar::Client unsafe({.application_id = "app-1", .publishable_key = "ks_pk_live_x", .base_url = "http://example.com"}, transport, deviceProvider, tokenStore);
	} catch (const std::invalid_argument&) { rejected = true; }
	if (!rejected) throw std::runtime_error("Client accepted public plaintext HTTP");

	transport->responses.push_back({.status_code = 401, .body = R"({"ok":false,"code":"INVALID_CREDENTIALS","message":"invalid"})"});
	keystar::Client client({.application_id = "app-1", .publishable_key = "ks_pk_live_x"}, transport, deviceProvider, tokenStore);
	const std::string email = "quote\"slash\\line\n@example.com";
	(void)client.login(email, "pass\"word\\value");
	if (transport->requests.empty()) throw std::runtime_error("login request was not sent");
	const auto json = keystar::JsonValue::parse(transport->requests[0].body);
	if (json.getString("email") != email || json.getString("password") != "pass\"word\\value") {
		throw std::runtime_error("Client emitted malformed or injectable JSON");
	}
	printf("  PASS testClientRejectsUnsafeBaseUrlAndEscapesJson\n");
}

void testRefreshSuccess() {
    auto transport = std::make_shared<FakeTransport>();
    auto deviceProvider = std::make_shared<FakeDeviceProvider>();
    auto tokenStore = std::make_shared<FakeTokenStore>();

    // Pre-populate the token store with a refresh token.
    tokenStore->savedSession = keystar::StoredSession{
        .refresh_token = "old-refresh-token",
        .user_id = "user-1",
        .device_id = "dev-1",
    };

    transport->responses.push_back({
        .status_code = 200,
        .body = R"({"ok":true,"access_token":"new-access","refresh_token":"new-refresh","token_expires_at":"2026-09-20T00:00:00Z"})"
    });

    keystar::Client client(
        {.application_id = "app-1", .publishable_key = "ks_pk_live_..."},
        transport, deviceProvider, tokenStore
    );

    auto result = client.refresh();
    assert(result.ok);
    assert(result.data.access_token == "new-access");
    assert(result.data.refresh_token == "new-refresh");

    // Verify the refresh request was made.
    assert(transport->requests.size() == 1);
	assert(transport->requests[0].url.find("/v1/auth/refresh") != std::string::npos);
	if (transport->requests[0].headers["Authorization"] != "Bearer ks_pk_live_...") {
		throw std::runtime_error("refresh request did not use the publishable credential");
	}

    printf("  PASS testRefreshSuccess\n");
}

void testRefreshWithoutToken() {
    auto transport = std::make_shared<FakeTransport>();
    auto deviceProvider = std::make_shared<FakeDeviceProvider>();
    auto tokenStore = std::make_shared<FakeTokenStore>();

    keystar::Client client(
        {.application_id = "app-1", .publishable_key = "ks_pk_live_..."},
        transport, deviceProvider, tokenStore
    );

    auto result = client.refresh();
    assert(!result.ok);
    assert(result.error_code == "INVALID_REFRESH_TOKEN");

    // No network requests should be made.
    assert(transport->requests.empty());

    printf("  PASS testRefreshWithoutToken\n");
}

void testLogout() {
    auto transport = std::make_shared<FakeTransport>();
    auto deviceProvider = std::make_shared<FakeDeviceProvider>();
    auto tokenStore = std::make_shared<FakeTokenStore>();

    tokenStore->savedSession = keystar::StoredSession{
        .refresh_token = "refresh-token",
    };

    keystar::Client client(
        {.application_id = "app-1", .publishable_key = "ks_pk_live_..."},
        transport, deviceProvider, tokenStore
    );

	if (!client.isAuthenticated() || !client.logout() || client.isAuthenticated() || tokenStore->savedSession.has_value()) {
		throw std::runtime_error("logout did not clear the local session");
	}
	if (transport->requests.size() != 1 || transport->requests[0].headers["Authorization"] != "Bearer ks_pk_live_...") {
		throw std::runtime_error("logout did not revoke using the publishable credential");
	}

    printf("  PASS testLogout\n");
}

}  // namespace

void run_auth_tests() {
    printf("Running auth tests...\n");
    testLoginSuccess();
	testLoginFailure();
	testClientRejectsUnsafeBaseUrlAndEscapesJson();
    testRefreshSuccess();
    testRefreshWithoutToken();
    testLogout();
    printf("  All auth tests passed.\n");
}
