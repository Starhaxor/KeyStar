#include "keystar/transport.hpp"

#include <curl/curl.h>

#include <memory>
#include <mutex>

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

        if (!curl_) {
            response.status_code = -1;
            return response;
        }

        curl_easy_reset(curl_);

        // Set URL.
        curl_easy_setopt(curl_, CURLOPT_URL, request.url.c_str());

        // Set method.
        switch (request.method) {
            case HttpMethod::Get:
                break;  // default is GET
            case HttpMethod::Post:
                curl_easy_setopt(curl_, CURLOPT_POST, 1L);
                curl_easy_setopt(curl_, CURLOPT_POSTFIELDS, request.body.c_str());
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
        struct curl_slist* headers = nullptr;
        for (const auto& [key, value] : request.headers) {
            std::string header = key + ": " + value;
            headers = curl_slist_append(headers, header.c_str());
        }
        if (headers) {
            curl_easy_setopt(curl_, CURLOPT_HTTPHEADER, headers);
        }

        // Response buffer.
        std::string body;
        curl_easy_setopt(curl_, CURLOPT_WRITEFUNCTION, writeCallback);
        curl_easy_setopt(curl_, CURLOPT_WRITEDATA, &body);

        // Execute.
        CURLcode res = curl_easy_perform(curl_);
        if (res != CURLE_OK) {
            response.status_code = -1;
        } else {
            long status = 0;
            curl_easy_getinfo(curl_, CURLINFO_RESPONSE_CODE, &status);
            response.status_code = static_cast<int>(status);
        }
        response.body = std::move(body);

        if (headers) curl_slist_free_all(headers);
        return response;
    }

private:
    static size_t writeCallback(char* ptr, size_t size, size_t nmemb, void* userdata) {
        auto* body = static_cast<std::string*>(userdata);
        body->append(ptr, size * nmemb);
        return size * nmemb;
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
