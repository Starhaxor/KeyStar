#include "keystar/token_store.hpp"

#include <mutex>
#include <optional>
#include <string>

namespace keystar {

/// InMemoryTokenStore is a fallback that keeps tokens only in process
/// memory. Tokens do not survive process restarts. Used when no
/// platform-specific secure storage is available.
class InMemoryTokenStore : public TokenStore {
public:
    bool save(const StoredSession& session) override {
        std::lock_guard lock(mutex_);
        session_ = session;
        return true;
    }

    std::optional<StoredSession> load() override {
        std::lock_guard lock(mutex_);
        if (session_.refresh_token.empty()) return std::nullopt;
        return session_;
    }

    bool clear() override {
        std::lock_guard lock(mutex_);
        session_ = StoredSession{};
        return true;
    }

private:
    std::mutex mutex_;
    StoredSession session_;
};

// Platform-specific factory (defined in platform/ source files).
// Falls back to InMemoryTokenStore when no secure storage is available.
std::shared_ptr<TokenStore> createPlatformTokenStore(const std::string& /*appId*/) {
    return std::make_shared<InMemoryTokenStore>();
}

}  // namespace keystar
