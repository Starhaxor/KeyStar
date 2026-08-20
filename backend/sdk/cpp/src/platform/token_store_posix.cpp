#ifndef _WIN32

#include "keystar/token_store.hpp"

// POSIX (Linux/macOS) token store implementation.
//
// macOS: Uses Security.framework Keychain (SecKeychainAddGenericPassword,
//   SecKeychainFindGenericPassword).
//
// Linux: Uses libsecret / Secret Service API
//   (secret_password_store / secret_password_lookup).
//
// This is a stub; falls back to in-memory when the platform API is not
// available.

namespace keystar {

std::shared_ptr<TokenStore> createPlatformTokenStore(const std::string& appId) {
    // TODO: Implement Keychain (macOS) or libsecret (Linux) token store.
    //
    // macOS:
    //   SecKeychainAddGenericPassword(kc, ...)
    //   SecKeychainFindGenericPassword(kc, ...)
    //
    // Linux:
    //   secret_password_store(SECRET_SCHEMA_DEFAULT, ...)
    //   secret_password_lookup(SECRET_SCHEMA_DEFAULT, ...)
    //
    // For now, use in-memory fallback.
    class PosixTokenStore : public TokenStore {
    public:
        explicit PosixTokenStore(const std::string& id) : appId_(id) {}
        bool save(const StoredSession&) override { return false; }
        std::optional<StoredSession> load() override { return std::nullopt; }
        bool clear() override { return true; }
    private:
        std::string appId_;
    };

    return std::make_shared<PosixTokenStore>(appId);
}

}  // namespace keystar

#endif  // !_WIN32
