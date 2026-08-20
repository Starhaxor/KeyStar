#include "keystar/token_store.hpp"

#include <cassert>
#include <cstdio>
#include <memory>

namespace {

/// InMemoryTokenStore for testing.
class InMemoryTokenStore : public keystar::TokenStore {
public:
    bool save(const keystar::StoredSession& s) override {
        session_ = s;
        return true;
    }

    std::optional<keystar::StoredSession> load() override {
        if (session_.refresh_token.empty()) return std::nullopt;
        return session_;
    }

    bool clear() override {
        session_ = keystar::StoredSession{};
        return true;
    }

private:
    keystar::StoredSession session_;
};

void testTokenStoreSaveAndLoad() {
    auto store = std::make_shared<InMemoryTokenStore>();

    keystar::StoredSession session;
    session.refresh_token = "refresh-abc";
    session.user_id = "user-1";
    session.device_id = "dev-1";
    session.license_id = "lic-1";
    session.expires_at = "2026-09-20T00:00:00Z";

    assert(store->save(session));

    auto loaded = store->load();
    assert(loaded.has_value());
    assert(loaded->refresh_token == "refresh-abc");
    assert(loaded->user_id == "user-1");
    assert(loaded->device_id == "dev-1");
    assert(loaded->license_id == "lic-1");

    printf("  PASS testTokenStoreSaveAndLoad\n");
}

void testTokenStoreLoadEmpty() {
    auto store = std::make_shared<InMemoryTokenStore>();

    auto loaded = store->load();
    assert(!loaded.has_value());

    printf("  PASS testTokenStoreLoadEmpty\n");
}

void testTokenStoreClear() {
    auto store = std::make_shared<InMemoryTokenStore>();

    keystar::StoredSession session;
    session.refresh_token = "refresh-xyz";
    store->save(session);
    assert(store->load().has_value());

    assert(store->clear());
    assert(!store->load().has_value());

    printf("  PASS testTokenStoreClear\n");
}

void testTokenStoreOverwrite() {
    auto store = std::make_shared<InMemoryTokenStore>();

    keystar::StoredSession first;
    first.refresh_token = "first";
    store->save(first);

    keystar::StoredSession second;
    second.refresh_token = "second";
    store->save(second);

    auto loaded = store->load();
    assert(loaded.has_value());
    assert(loaded->refresh_token == "second");

    printf("  PASS testTokenStoreOverwrite\n");
}

}  // namespace

void run_token_store_tests() {
    printf("Running token store tests...\n");
    testTokenStoreSaveAndLoad();
    testTokenStoreLoadEmpty();
    testTokenStoreClear();
    testTokenStoreOverwrite();
    printf("  All token store tests passed.\n");
}
