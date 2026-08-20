#pragma once

#include <map>
#include <string>

namespace keystar {

/// Lightweight JSON value for parsing backend responses. Only supports the
/// subset needed by the SDK: objects, strings, numbers, bools and arrays
/// of strings.
class JsonValue {
public:
    enum Type { Null, Bool, Number, String, Array, Object };

    JsonValue() : type_(Null) {}
    explicit JsonValue(std::string raw);

    Type type() const noexcept { return type_; }
    bool isNull() const noexcept { return type_ == Null; }

    // Accessors (return empty/0/false for type mismatches)
    bool boolValue() const;
    int64_t intValue() const;
    double doubleValue() const;
    const std::string& stringValue() const;
    const std::vector<JsonValue>& arrayValue() const;
    const std::map<std::string, JsonValue>& objectValue() const;

    // Object field access with dot notation: "user.email"
    JsonValue get(const std::string& path) const;
    std::string getString(const std::string& path, const std::string& def = "") const;
    bool getBool(const std::string& path, bool def = false) const;
    int64_t getInt(const std::string& path, int64_t def = 0) const;

    // Check if a field exists
    bool has(const std::string& key) const;

    // Parse a JSON string. Returns a null JsonValue on parse error.
    static JsonValue parse(const std::string& json);

private:
    Type type_;
    bool boolVal_ = false;
    int64_t intVal_ = 0;
    double doubleVal_ = 0.0;
    std::string stringVal_;
    std::vector<JsonValue> arrayVal_;
    std::map<std::string, JsonValue> objectVal_;

    void parseImpl(const char*& pos, const char* end);
    void parseValue(const char*& pos, const char* end);
    void parseString(const char*& pos, const char* end);
    void parseNumber(const char*& pos, const char* end);
    void parseArray(const char*& pos, const char* end);
    void parseObject(const char*& pos, const char* end);
    void skipWhitespace(const char*& pos, const char* end);
};

}  // namespace keystar
