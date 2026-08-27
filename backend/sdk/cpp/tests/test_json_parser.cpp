#include "keystar/json_parser.hpp"

#include <cassert>
#include <cstdio>

namespace {

void testJsonParseObject() {
    auto json = keystar::JsonValue::parse(R"({"ok":true,"name":"test","count":42})");
    assert(json.has("ok"));
    assert(json.getBool("ok") == true);
    assert(json.getString("name") == "test");
    assert(json.getInt("count") == 42);
    printf("  PASS testJsonParseObject\n");
}

void testJsonParseNested() {
    auto json = keystar::JsonValue::parse(R"({"user":{"email":"a@b.c","id":"123"}})");
    assert(json.getString("user.email") == "a@b.c");
    assert(json.getString("user.id") == "123");
    printf("  PASS testJsonParseNested\n");
}

void testJsonParseEmpty() {
    auto json = keystar::JsonValue::parse("");
    assert(json.isNull());
    printf("  PASS testJsonParseEmpty\n");
}

void testJsonParseInvalid() {
    const char* invalid[] = {
        "{invalid}", "{\"key\" 1}", "{\"key\":true} trailing", "{\"key\":1,}", "[1,]",
        "tru", "nullx", "01", "\"unterminated", "{\"key\":1,\"key\":2}"
    };
    for (const char* input : invalid) {
        auto json = keystar::JsonValue::parse(input);
        assert(json.isNull());
    }
    printf("  PASS testJsonParseInvalid\n");
}

void testJsonParseArray() {
    auto json = keystar::JsonValue::parse(R"({"items":["a","b","c"]})");
    assert(json.has("items"));
    assert(json.arrayValue().size() == 0);  // top-level is object
    auto items = json.get("items");
    assert(items.arrayValue().size() == 3);
    printf("  PASS testJsonParseArray\n");
}

void testJsonParseErrorResponse() {
    auto json = keystar::JsonValue::parse(R"({"ok":false,"code":"LICENSE_EXPIRED","message":"license has expired"})");
    assert(json.getBool("ok") == false);
    assert(json.getString("code") == "LICENSE_EXPIRED");
    assert(json.getString("message") == "license has expired");
    printf("  PASS testJsonParseErrorResponse\n");
}

void testJsonGetDefault() {
    auto json = keystar::JsonValue::parse(R"({"ok":true})");
    assert(json.getString("missing", "default") == "default");
    assert(json.getBool("missing", true) == true);
    assert(json.getInt("missing", 99) == 99);
    printf("  PASS testJsonGetDefault\n");
}

}  // namespace

void run_json_parser_tests() {
    printf("Running JSON parser tests...\n");
    testJsonParseObject();
    testJsonParseNested();
    testJsonParseEmpty();
    testJsonParseInvalid();
    testJsonParseArray();
    testJsonParseErrorResponse();
    testJsonGetDefault();
    printf("  All JSON parser tests passed.\n");
}
