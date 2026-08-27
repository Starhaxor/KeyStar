#include "keystar/transport.hpp"

#include <curl/curl.h>

#include <memory>
#include <mutex>

namespace {
constexpr std::size_t kMaxResponseBytes = 1024 * 1024;
struct ResponseBuffer { std::string body; bool overflow = false; };

bool allowedUrl(const std::string& url, bool allowLoopback) {
	if (url.rfind("https://", 0) == 0) {
		const auto end = url.find('/', 8);
		const auto authority = url.substr(8, end - 8);
		return !authority.empty() && authority.find('@') == std::string::npos;
	}
    if (!allowLoopback || url.rfind("http://", 0) != 0) return false;
    const auto end = url.find('/', 7);
    const auto authority = url.substr(7, end - 7);
    return authority == "localhost" || authority.rfind("localhost:", 0) == 0 ||
           authority == "127.0.0.1" || authority.rfind("127.0.0.1:", 0) == 0 ||
           authority == "[::1]" || authority.rfind("[::1]:", 0) == 0;
}
}

namespace keystar {

/// CurlTransport is a cross-platform HTTP transport using libcurl.
class CurlTransport : public Transport {
public:
    CurlTransport() {
        curl_ = curl_easy_init();
    }

    ~CurlTransport() override {
        if (curl_) curl_easy_cleanup(curl_);
    }

    CurlTransport(const CurlTransport&) = delete;
    CurlTransport& operator=(const CurlTransport&) = delete;

    HttpResponse send(const HttpRequest& request) override {
        HttpResponse response;

		if (!curl_ || !allowedUrl(request.url, request.allow_insecure_loopback)) {
            response.status_code = -1;
            return response;
        }

        curl_easy_reset(curl_);

        // Set URL.
		curl_easy_setopt(curl_, CURLOPT_URL, request.url.c_str());
		curl_easy_setopt(curl_, CURLOPT_PROTOCOLS, request.allow_insecure_loopback ? CURLPROTO_HTTP | CURLPROTO_HTTPS : CURLPROTO_HTTPS);
		curl_easy_setopt(curl_, CURLOPT_REDIR_PROTOCOLS, CURLPROTO_HTTPS);
		curl_easy_setopt(curl_, CURLOPT_FOLLOWLOCATION, 0L);
		curl_easy_setopt(curl_, CURLOPT_CONNECTTIMEOUT_MS, 5000L);
		curl_easy_setopt(curl_, CURLOPT_TIMEOUT_MS, 10000L);
		curl_easy_setopt(curl_, CURLOPT_SSL_VERIFYPEER, 1L);
		curl_easy_setopt(curl_, CURLOPT_SSL_VERIFYHOST, 2L);
		curl_easy_setopt(curl_, CURLOPT_SSLVERSION, CURL_SSLVERSION_TLSv1_2);
		curl_easy_setopt(curl_, CURLOPT_POSTFIELDSIZE_LARGE, static_cast<curl_off_t>(request.body.size()));

        // Set method.
        switch (request.method) {
            case HttpMethod::Get:
                break;  // default is GET
            case HttpMethod::Post:
                curl_easy_setopt(curl_, CURLOPT_POST, 1L);
				curl_easy_setopt(curl_, CURLOPT_POSTFIELDS, request.body.data());
				curl_easy_setopt(curl_, CURLOPT_POSTFIELDSIZE_LARGE, static_cast<curl_off_t>(request.body.size()));
                break;
            case HttpMethod::Put:
                curl_easy_setopt(curl_, CURLOPT_CUSTOMREQUEST, "PUT");
                curl_easy_setopt(curl_, CURLOPT_POSTFIELDS, request.body.c_str());
                break;
            case HttpMethod::Patch:
                curl_easy_setopt(curl_, CURLOPT_CUSTOMREQUEST, "PATCH");
                curl_easy_setopt(curl_, CURLOPT_POSTFIELDS, request.body.c_str());
                break;
            case HttpMethod::Delete:
                curl_easy_setopt(curl_, CURLOPT_CUSTOMREQUEST, "DELETE");
                break;
        }

        // Set headers.
		for (const auto& [key, value] : request.headers) {
			if (key.find_first_of("\r\n") != std::string::npos || value.find_first_of("\r\n") != std::string::npos) {
				response.status_code = -1;
				return response;
			}
		}
		struct curl_slist* headers = nullptr;
		for (const auto& [key, value] : request.headers) {
			std::string header = key + ": " + value;
            headers = curl_slist_append(headers, header.c_str());
        }
        if (headers) {
            curl_easy_setopt(curl_, CURLOPT_HTTPHEADER, headers);
        }

        // Response buffer.
		ResponseBuffer buffer;
		curl_easy_setopt(curl_, CURLOPT_WRITEFUNCTION, writeCallback);
		curl_easy_setopt(curl_, CURLOPT_WRITEDATA, &buffer);

        // Execute.
        CURLcode res = curl_easy_perform(curl_);
        if (res != CURLE_OK) {
            response.status_code = -1;
        } else {
            long status = 0;
            curl_easy_getinfo(curl_, CURLINFO_RESPONSE_CODE, &status);
            response.status_code = static_cast<int>(status);
        }
		response.body = std::move(buffer.body);
		if (buffer.overflow) response.status_code = -1;

        if (headers) curl_slist_free_all(headers);
        return response;
    }

private:
    static size_t writeCallback(char* ptr, size_t size, size_t nmemb, void* userdata) {
		auto* buffer = static_cast<ResponseBuffer*>(userdata);
		const auto bytes = size * nmemb;
		if (bytes > kMaxResponseBytes - buffer->body.size()) { buffer->overflow = true; return 0; }
		buffer->body.append(ptr, bytes);
		return bytes;
    }

    CURL* curl_ = nullptr;
};

// Factory function to create the default transport.
std::shared_ptr<Transport> createDefaultTransport() {
    static std::once_flag flag;
    std::call_once(flag, []() { curl_global_init(CURL_GLOBAL_DEFAULT); });
    return std::make_shared<CurlTransport>();
}

}  // namespace keystar
