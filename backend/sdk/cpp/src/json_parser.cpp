#include "keystar/json_parser.hpp"

#include <cstdlib>
#include <cstring>
#include <stdexcept>

namespace keystar {

JsonValue::JsonValue(std::string raw) : type_(Null) {
    const char* pos = raw.c_str();
    const char* end = raw.c_str() + raw.size();
    parseValue(pos, end);
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

void JsonValue::parseValue(const char*& pos, const char* end) {
    skipWhitespace(pos, end);
    if (pos >= end) return;

    char c = *pos;
    if (c == '"') {
        parseString(pos, end);
    } else if (c == '{') {
        parseObject(pos, end);
    } else if (c == '[') {
        parseArray(pos, end);
    } else if (c == 't' || c == 'f') {
        type_ = Bool;
        boolVal_ = (c == 't');
        pos += (c == 't') ? 4 : 5;
    } else if (c == 'n') {
        type_ = Null;
        pos += 4;
    } else {
        parseNumber(pos, end);
    }
}

void JsonValue::parseString(const char*& pos, const char* end) {
    type_ = String;
    ++pos;  // skip opening quote
    std::string result;
    while (pos < end && *pos != '"') {
        if (*pos == '\\') {
            ++pos;
            if (pos >= end) break;
            switch (*pos) {
                case '"': result += '"'; break;
                case '\\': result += '\\'; break;
                case '/': result += '/'; break;
                case 'n': result += '\n'; break;
                case 'r': result += '\r'; break;
                case 't': result += '\t'; break;
                case 'u': {
                    // Simple 4-hex-digit unicode escape (ASCII only for now)
                    if (pos + 4 < end) {
                        char hex[5] = {};
                        std::memcpy(hex, pos + 1, 4);
                        result += static_cast<char>(std::strtol(hex, nullptr, 16));
                        pos += 4;
                    }
                    break;
                }
                default: result += *pos; break;
            }
        } else {
            result += *pos;
        }
        ++pos;
    }
    if (pos < end) ++pos;  // skip closing quote
    stringVal_ = std::move(result);
}

void JsonValue::parseNumber(const char*& pos, const char* end) {
    type_ = Number;
    const char* start = pos;
    while (pos < end && (*pos == '-' || *pos == '+' || *pos == '.' ||
           (*pos >= '0' && *pos <= '9') || *pos == 'e' || *pos == 'E')) {
        ++pos;
    }
    std::string numStr(start, pos);
    if (numStr.find('.') != std::string::npos || numStr.find('e') != std::string::npos ||
        numStr.find('E') != std::string::npos) {
        doubleVal_ = std::strtod(numStr.c_str(), nullptr);
        intVal_ = static_cast<int64_t>(doubleVal_);
    } else {
        intVal_ = std::strtoll(numStr.c_str(), nullptr, 10);
        doubleVal_ = static_cast<double>(intVal_);
    }
}

void JsonValue::parseArray(const char*& pos, const char* end) {
    type_ = Array;
    ++pos;  // skip '['
    skipWhitespace(pos, end);
    if (pos < end && *pos == ']') { ++pos; return; }
    while (pos < end) {
        JsonValue item;
        item.parseValue(pos, end);
        arrayVal_.push_back(std::move(item));
        skipWhitespace(pos, end);
        if (pos < end && *pos == ',') { ++pos; continue; }
        break;
    }
    skipWhitespace(pos, end);
    if (pos < end) ++pos;  // skip ']'
}

void JsonValue::parseObject(const char*& pos, const char* end) {
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
        if (pos < end) ++pos;  // skip ':'
        JsonValue val;
        val.parseValue(pos, end);
        objectVal_[keyVal.stringVal_] = std::move(val);
        skipWhitespace(pos, end);
        if (pos < end && *pos == ',') { ++pos; continue; }
        break;
    }
    skipWhitespace(pos, end);
    if (pos < end) ++pos;  // skip '}'
}

}  // namespace keystar
