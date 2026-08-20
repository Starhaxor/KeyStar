#pragma once

#include <memory>
#include <mutex>

#include "types.hpp"
#include "error.hpp"
#include "transport.hpp"
#include "device_identity.hpp"
#include "token_store.hpp"

namespace keystar {

/// Client is the main entry point for the KeyStar C++ SDK. It manages
/// authentication, device verification, token refresh and user profile
/// retrieval.
///
/// Usage:
///   keystar::Client client({
///       .application_id = "...",
///       .publishable_key = "ks_pk_live_...",
///   });
///
///   auto result = client.login(email, password);
///   if (result.ok()) { ... }
///
/// Thread safety: The Client is NOT thread-safe. Callers must synchronize
/// access or use one Client per thread. Token refresh is serialized via
/// an internal mutex.
class Client {
public:
    /// Construct with default implementations for all dependencies.
    explicit Client(ClientOptions options);

    /// Construct with explicit dependency injection for testing.
    Client(ClientOptions options,
           std::shared_ptr<Transport> transport,
           std::shared_ptr<DeviceIdentityProvider> deviceProvider,
           std::shared_ptr<TokenStore> tokenStore);

    /// Login with email and password. Handles the full flow:
    ///   1. Collect device identity
    ///   2. POST /v1/auth/login
    ///   3. Sign challenge
    ///   4. POST /v1/device/verify
    ///   5. Store refresh token
    /// Returns SessionResult on success.
    ApiResponse<SessionResult> login(const std::string& email,
                                     const std::string& password);

    /// Refresh the access token using the stored refresh token.
    ApiResponse<SessionResult> refresh();

    /// Retrieve the current user profile from /v1/me.
    ApiResponse<UserProfile> me();

    /// Logout: revoke refresh token and clear local storage.
    bool logout();

    /// Returns true if a valid session exists (access token or refresh token).
    bool isAuthenticated() const;

    /// Access the current access token (empty if not authenticated).
    std::string accessToken() const;

private:
    ClientOptions options_;
    std::shared_ptr<Transport> transport_;
    std::shared_ptr<DeviceIdentityProvider> deviceProvider_;
    std::shared_ptr<TokenStore> tokenStore_;

    mutable std::mutex mutex_;
    std::string accessToken_;
    std::string refreshToken_;
    std::string tokenExpiresAt_;
    std::string userId_;
    std::string deviceId_;
    std::string licenseId_;

    // Internal helpers
    std::string buildUrl(const std::string& path) const;
    HttpRequest makeRequest(HttpMethod method, const std::string& path,
                            const std::string& body = "") const;
    void storeSession(const SessionResult& session);
    void clearSession();
};

}  // namespace keystar
