#pragma once

#include <map>
#include <string>

namespace keystar {

/// HTTP request method.
enum class HttpMethod {
    Get,
    Post,
    Patch,
    Put,
    Delete,
};

/// A single HTTP request.
struct HttpRequest {
    HttpMethod method = HttpMethod::Get;
    std::string url;
    std::map<std::string, std::string> headers;
	std::string body;  // JSON body for POST/PUT/PATCH
	bool allow_insecure_loopback = false;
};

/// A single HTTP response.
struct HttpResponse {
    int status_code = 0;
    std::map<std::string, std::string> headers;
    std::string body;
    bool ok() const noexcept { return status_code >= 200 && status_code < 300; }
};

/// Transport is an abstract interface for sending HTTP requests. This allows
/// dependency injection for testing with a fake transport.
class Transport {
public:
    virtual ~Transport() = default;
    virtual HttpResponse send(const HttpRequest& request) = 0;
};

}  // namespace keystar
