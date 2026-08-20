#pragma once

#include <optional>
#include <string>

namespace keystar {

/// StoredSession holds the persistent session data that survives process
/// restarts. The access token is never stored; only the refresh token and
/// associated metadata are persisted.
struct StoredSession {
    std::string refresh_token;
    std::string user_id;
    std::string device_id;
    std::string license_id;
    std::string expires_at;  // RFC3339
};

/// TokenStore is an abstract interface for securely persisting refresh tokens.
/// Implementations use platform-specific secure storage:
///   - Windows: DPAPI / Credential Manager
///   - macOS: Keychain
///   - Linux: Secret Service / libsecret
///
/// If no secure storage is available, the SDK stores tokens only in memory
/// (ephemeral mode) unless the developer explicitly opts in.
class TokenStore {
public:
    virtual ~TokenStore() = default;

    /// Save a session to secure storage. Overwrites any existing session.
    virtual bool save(const StoredSession& session) = 0;

    /// Load the previously saved session. Returns std::nullopt if no
    /// session is stored or if decryption fails.
    virtual std::optional<StoredSession> load() = 0;

    /// Delete the stored session (logout).
    virtual bool clear() = 0;
};

}  // namespace keystar
