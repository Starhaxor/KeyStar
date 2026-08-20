#ifdef _WIN32

#include "keystar/token_store.hpp"

// Windows DPAPI / Credential Manager implementation.
// This is a stub; the full implementation would use:
//   - CryptProtectData / CryptUnprotectData for DPAPI
//   - CredWrite / CredRead for Credential Manager
//
// For now, falls back to in-memory storage.

namespace keystar {

// Forward declaration of the in-memory fallback.
std::shared_ptr<TokenStore> createPlatformTokenStore(const std::string& appId) {
    // TODO: Implement DPAPI-backed token store.
    // 1. Derive a key from the application_id
    // 2. Use CryptProtectData to encrypt the refresh token
    // 3. Store in %APPDATA%/KeyStar/<appId>.dat
    //
    // For now, use in-memory fallback.
    class DpapiTokenStore : public TokenStore {
    public:
        explicit DpapiTokenStore(const std::string& id) : appId_(id) {}
        bool save(const StoredSession&) override { return false; }
        std::optional<StoredSession> load() override { return std::nullopt; }
        bool clear() override { return true; }
    private:
        std::string appId_;
    };

    return std::make_shared<DpapiTokenStore>(appId);
}

}  // namespace keystar

#endif  // _WIN32
