#include "keystar/device_identity.hpp"
#include "keystar/json_parser.hpp"

namespace keystar {

// Device verification is handled by the Client::login() flow. The
// DeviceIdentityProvider handles challenge signing, and the Client sends
// the signed challenge to POST /v1/device/verify.
//
// This file contains any helper utilities for device verification, such
// as base64 encoding/decoding for the challenge and signature.

// Base64 encode/decode utilities for the SDK.
static const char base64Chars[] =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

std::string base64Encode(const uint8_t* data, size_t len) {
    std::string result;
    result.reserve(((len + 2) / 3) * 4);
    for (size_t i = 0; i < len; i += 3) {
        uint32_t n = static_cast<uint32_t>(data[i]) << 16;
        if (i + 1 < len) n |= static_cast<uint32_t>(data[i + 1]) << 8;
        if (i + 2 < len) n |= static_cast<uint32_t>(data[i + 2]);

        result += base64Chars[(n >> 18) & 0x3F];
        result += base64Chars[(n >> 12) & 0x3F];
        result += (i + 1 < len) ? base64Chars[(n >> 6) & 0x3F] : '=';
        result += (i + 2 < len) ? base64Chars[n & 0x3F] : '=';
    }
    return result;
}

std::vector<uint8_t> base64Decode(const std::string& encoded) {
    std::vector<uint8_t> result;
    result.reserve(encoded.size() * 3 / 4);

    // Build reverse lookup table
    int lookup[256] = {};
    std::fill_n(&lookup[0], 256, -1);
    for (int i = 0; base64Chars[i]; ++i) {
        lookup[static_cast<unsigned char>(base64Chars[i])] = i;
    }

    uint32_t accumulator = 0;
    int bits = 0;
    for (char c : encoded) {
        if (c == '=') break;
        int val = lookup[static_cast<unsigned char>(c)];
        if (val < 0) continue;
        accumulator = (accumulator << 6) | val;
        bits += 6;
        if (bits >= 8) {
            bits -= 8;
            result.push_back(static_cast<uint8_t>((accumulator >> bits) & 0xFF));
        }
    }
    return result;
}

}  // namespace keystar
