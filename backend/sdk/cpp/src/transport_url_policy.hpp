#pragma once
#include <string>

namespace keystar::detail {
inline bool allowedTransportURL(const std::string& url, bool allowLoopback) {
    if (url.find_first_of("@\\\r\n\t ") != std::string::npos || url.find('\0') != std::string::npos) return false;
    const bool secure = url.rfind("https://", 0) == 0;
    if (!secure && (!allowLoopback || url.rfind("http://", 0) != 0)) return false;
    const std::size_t start = secure ? 8 : 7;
    const auto end = url.find_first_of("/?#", start);
    const auto authority = url.substr(start, end - start);
    if (authority.empty()) return false;
    if (secure) return true;
    for (const std::string host : {"localhost", "127.0.0.1", "[::1]"}) {
        if (authority == host) return true;
        if (authority.rfind(host + ":", 0) == 0) {
            const auto port = authority.substr(host.size() + 1);
            return !port.empty() && port.size() <= 5 && port.find_first_not_of("0123456789") == std::string::npos
                && std::stoul(port) > 0 && std::stoul(port) <= 65535;
        }
    }
    return false;
}
}
