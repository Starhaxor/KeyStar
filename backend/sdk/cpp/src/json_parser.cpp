#include "keystar/json_parser.hpp"

#include <cerrno>
#include <cmath>
#include <cstdlib>
#include <cstring>
#include <limits>
#include <stdexcept>

namespace keystar {

JsonValue::JsonValue(std::string raw) : type_(Null) {
    const char* pos = raw.c_str();
    const char* end = raw.c_str() + raw.size();
    parseValue(pos, end);
    skipWhitespace(pos, end);
    if (pos != end) throw std::runtime_error("trailing JSON data");
}

JsonValue JsonValue::parse(const std::string& json) {
    try {
        return JsonValue(json);
    } catch (...) {
        return JsonValue();
    }
}

bool JsonValue::boolValue() const { return boolVal_; }
int64_t JsonValue::intValue() const { return intVal_; }
double JsonValue::doubleValue() const { return doubleVal_; }
const std::string& JsonValue::stringValue() const { return stringVal_; }
const std::vector<JsonValue>& JsonValue::arrayValue() const { return arrayVal_; }
const std::map<std::string, JsonValue>& JsonValue::objectValue() const { return objectVal_; }

JsonValue JsonValue::get(const std::string& path) const {
    if (type_ != Object) return JsonValue();

    size_t dot = path.find('.');
    if (dot == std::string::npos) {
        auto it = objectVal_.find(path);
        return it != objectVal_.end() ? it->second : JsonValue();
    }
    std::string key = path.substr(0, dot);
    auto it = objectVal_.find(key);
    if (it == objectVal_.end()) return JsonValue();
    return it->second.get(path.substr(dot + 1));
}

std::string JsonValue::getString(const std::string& path, const std::string& def) const {
    JsonValue v = get(path);
    return v.type_ == String ? v.stringVal_ : def;
}

bool JsonValue::getBool(const std::string& path, bool def) const {
    JsonValue v = get(path);
    return v.type_ == Bool ? v.boolVal_ : def;
}

int64_t JsonValue::getInt(const std::string& path, int64_t def) const {
    JsonValue v = get(path);
    return v.type_ == Number ? v.intVal_ : def;
}

bool JsonValue::has(const std::string& key) const {
    return type_ == Object && objectVal_.count(key) > 0;
}

void JsonValue::skipWhitespace(const char*& pos, const char* end) {
    while (pos < end && (*pos == ' ' || *pos == '\t' || *pos == '\n' || *pos == '\r')) {
        ++pos;
    }
}

void JsonValue::parseValue(const char*& pos, const char* end, std::size_t depth) {
    if (depth > 64) throw std::runtime_error("JSON nesting limit exceeded");
    skipWhitespace(pos, end);
    if (pos >= end) throw std::runtime_error("unexpected end of JSON");

    char c = *pos;
    if (c == '"') {
        parseString(pos, end);
    } else if (c == '{') {
        parseObject(pos, end, depth);
    } else if (c == '[') {
        parseArray(pos, end, depth);
    } else if (c == 't') {
        if (end - pos < 4 || std::memcmp(pos, "true", 4) != 0) {
            throw std::runtime_error("invalid JSON literal");
        }
        type_ = Bool;
        boolVal_ = true;
        pos += 4;
    } else if (c == 'f') {
        if (end - pos < 5 || std::memcmp(pos, "false", 5) != 0) {
            throw std::runtime_error("invalid JSON literal");
        }
        type_ = Bool;
        boolVal_ = false;
        pos += 5;
    } else if (c == 'n') {
        if (end - pos < 4 || std::memcmp(pos, "null", 4) != 0) {
            throw std::runtime_error("invalid JSON literal");
        }
        type_ = Null;
        pos += 4;
    } else if (c == '-' || (c >= '0' && c <= '9')) {
        parseNumber(pos, end);
    } else {
        throw std::runtime_error("invalid JSON value");
    }
}

void JsonValue::parseString(const char*& pos, const char* end) {
    ++pos;  // skip opening quote
    std::string result;
    while (pos < end && *pos != '"') {
        if (static_cast<unsigned char>(*pos) < 0x20) {
            throw std::runtime_error("unescaped control character in JSON string");
        }
        if (*pos == '\\') {
            ++pos;
            if (pos >= end) throw std::runtime_error("unterminated JSON escape");
            switch (*pos) {
                case '"': result += '"'; break;
                case '\\': result += '\\'; break;
                case '/': result += '/'; break;
                case 'b': result += '\b'; break;
                case 'f': result += '\f'; break;
                case 'n': result += '\n'; break;
                case 'r': result += '\r'; break;
                case 't': result += '\t'; break;
                case 'u': {
                    if (end - pos < 5) throw std::runtime_error("short JSON unicode escape");
                    unsigned int codepoint = 0;
                    for (int i = 1; i <= 4; ++i) {
                        const char h = pos[i];
                        codepoint <<= 4;
                        if (h >= '0' && h <= '9') codepoint |= static_cast<unsigned int>(h - '0');
                        else if (h >= 'a' && h <= 'f') codepoint |= static_cast<unsigned int>(h - 'a' + 10);
                        else if (h >= 'A' && h <= 'F') codepoint |= static_cast<unsigned int>(h - 'A' + 10);
                        else throw std::runtime_error("invalid JSON unicode escape");
                    }
                    if (codepoint >= 0xD800 && codepoint <= 0xDFFF) {
                        throw std::runtime_error("JSON surrogate escapes are unsupported");
                    }
                    if (codepoint <= 0x7F) result += static_cast<char>(codepoint);
                    else if (codepoint <= 0x7FF) {
                        result += static_cast<char>(0xC0 | (codepoint >> 6));
                        result += static_cast<char>(0x80 | (codepoint & 0x3F));
                    } else {
                        result += static_cast<char>(0xE0 | (codepoint >> 12));
                        result += static_cast<char>(0x80 | ((codepoint >> 6) & 0x3F));
                        result += static_cast<char>(0x80 | (codepoint & 0x3F));
                    }
                    pos += 4;
                    break;
                }
                default: throw std::runtime_error("invalid JSON escape");
            }
        } else {
            result += *pos;
        }
        ++pos;
    }
    if (pos >= end) throw std::runtime_error("unterminated JSON string");
    ++pos;  // skip closing quote
    type_ = String;
    stringVal_ = std::move(result);
}

void JsonValue::parseNumber(const char*& pos, const char* end) {
    const char* start = pos;
    if (*pos == '-') ++pos;
    if (pos >= end) throw std::runtime_error("invalid JSON number");
    if (*pos == '0') {
        ++pos;
        if (pos < end && *pos >= '0' && *pos <= '9') throw std::runtime_error("leading zero in JSON number");
    } else if (*pos >= '1' && *pos <= '9') {
        while (pos < end && *pos >= '0' && *pos <= '9') ++pos;
    } else {
        throw std::runtime_error("invalid JSON number");
    }
    if (pos < end && *pos == '.') {
        ++pos;
        if (pos >= end || *pos < '0' || *pos > '9') throw std::runtime_error("invalid JSON fraction");
        while (pos < end && *pos >= '0' && *pos <= '9') ++pos;
    }
    if (pos < end && (*pos == 'e' || *pos == 'E')) {
        ++pos;
        if (pos < end && (*pos == '+' || *pos == '-')) ++pos;
        if (pos >= end || *pos < '0' || *pos > '9') throw std::runtime_error("invalid JSON exponent");
        while (pos < end && *pos >= '0' && *pos <= '9') ++pos;
    }
    std::string numStr(start, pos);
    errno = 0;
    char* parsedEnd = nullptr;
    doubleVal_ = std::strtod(numStr.c_str(), &parsedEnd);
    if (errno == ERANGE || parsedEnd != numStr.c_str() + numStr.size() || !std::isfinite(doubleVal_)) {
        throw std::runtime_error("JSON number out of range");
    }
    if (numStr.find('.') != std::string::npos || numStr.find('e') != std::string::npos ||
        numStr.find('E') != std::string::npos) {
        constexpr double int64LowerBound = -9223372036854775808.0;
        constexpr double int64UpperBoundExclusive = 9223372036854775808.0;
        if (doubleVal_ >= int64LowerBound && doubleVal_ < int64UpperBoundExclusive) {
            intVal_ = static_cast<int64_t>(doubleVal_);
        }
    } else {
        errno = 0;
        intVal_ = std::strtoll(numStr.c_str(), &parsedEnd, 10);
        if (errno == ERANGE || parsedEnd != numStr.c_str() + numStr.size()) {
            throw std::runtime_error("JSON integer out of range");
        }
        doubleVal_ = static_cast<double>(intVal_);
    }
    type_ = Number;
}

void JsonValue::parseArray(const char*& pos, const char* end, std::size_t depth) {
    type_ = Array;
    ++pos;  // skip '['
    skipWhitespace(pos, end);
    if (pos < end && *pos == ']') { ++pos; return; }
    while (pos < end) {
        JsonValue item;
        item.parseValue(pos, end, depth + 1);
        arrayVal_.push_back(std::move(item));
        skipWhitespace(pos, end);
        if (pos < end && *pos == ',') {
            ++pos;
            skipWhitespace(pos, end);
            if (pos >= end || *pos == ']') throw std::runtime_error("trailing comma in JSON array");
            continue;
        }
        break;
    }
    skipWhitespace(pos, end);
    if (pos >= end || *pos != ']') throw std::runtime_error("unterminated JSON array");
    ++pos;
}

void JsonValue::parseObject(const char*& pos, const char* end, std::size_t depth) {
    type_ = Object;
    ++pos;  // skip '{'
    skipWhitespace(pos, end);
    if (pos < end && *pos == '}') { ++pos; return; }
    while (pos < end) {
        skipWhitespace(pos, end);
        if (pos >= end || *pos != '"') break;
        JsonValue keyVal;
        keyVal.parseString(pos, end);
        skipWhitespace(pos, end);
        if (pos >= end || *pos != ':') throw std::runtime_error("missing JSON object colon");
        ++pos;
        JsonValue val;
        val.parseValue(pos, end, depth + 1);
        if (!objectVal_.emplace(keyVal.stringVal_, std::move(val)).second) {
            throw std::runtime_error("duplicate JSON object key");
        }
        skipWhitespace(pos, end);
        if (pos < end && *pos == ',') {
            ++pos;
            skipWhitespace(pos, end);
            if (pos >= end || *pos == '}') throw std::runtime_error("trailing comma in JSON object");
            continue;
        }
        break;
    }
    skipWhitespace(pos, end);
    if (pos >= end || *pos != '}') throw std::runtime_error("unterminated JSON object");
    ++pos;
}

}  // namespace keystar
