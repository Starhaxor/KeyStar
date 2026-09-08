#include "keystar/client.hpp"
#include "../src/transport_url_policy.hpp"
#include <cstdlib>

#include <cassert>
#include <cstdio>
#include <stdexcept>
#include <string>
#include <vector>

namespace {

/// FakeTransport records requests and returns pre-configured responses.
class FakeTransport : public keystar::Transport {
public:
    struct RecordedRequest {
        keystar::HttpMethod method;
        std::string url;
        std::string body;
    };

    std::vector<RecordedRequest> requests;
    std::vector<keystar::HttpResponse> responses;

    keystar::HttpResponse send(const keystar::HttpRequest& request) override {
        requests.push_back({request.method, request.url, request.body});
        if (!responses.empty()) {
            auto resp = responses.front();
            responses.erase(responses.begin());
            return resp;
        }
        return {.status_code = 200, .body = "{\"ok\":true}"};
    }
};

void testFakeTransportRecordsRequests() {
    auto transport = std::make_shared<FakeTransport>();
    transport->responses.push_back({.status_code = 200, .body = "{\"ok\":true}"});

    keystar::HttpRequest req;
    req.method = keystar::HttpMethod::Post;
    req.url = "https://api.example.com/v1/auth/login";
    req.body = "{\"email\":\"test@example.com\"}";

    auto resp = transport->send(req);
    assert(resp.status_code == 200);
    assert(transport->requests.size() == 1);
    assert(transport->requests[0].method == keystar::HttpMethod::Post);
    assert(transport->requests[0].body == "{\"email\":\"test@example.com\"}");

    printf("  PASS testFakeTransportRecordsRequests\n");
}

void testHttpResponseOk() {
    keystar::HttpResponse ok{.status_code = 200};
    assert(ok.ok());

    keystar::HttpResponse created{.status_code = 201};
    assert(created.ok());

    keystar::HttpResponse badRequest{.status_code = 400};
    assert(!badRequest.ok());

    keystar::HttpResponse serverError{.status_code = 500};
    assert(!serverError.ok());

    printf("  PASS testHttpResponseOk\n");
}

#ifdef _WIN32
void testDefaultWindowsTransportIsAvailable() {
    if (!keystar::createDefaultTransport()) {
        throw std::runtime_error("default Windows transport is unavailable");
    }

    printf("  PASS testDefaultWindowsTransportIsAvailable\n");
}

void testDefaultWindowsTransportReportsUnsupportedUrl() {
    auto transport = keystar::createDefaultTransport();
    const auto response = transport->send({.url = "ftp://localhost/"});

    if (response.status_code != -1 ||
        response.body != R"({"code":"TRANSPORT_ERROR","message":"WinHTTP request failed"})") {
        throw std::runtime_error("default Windows transport did not return TRANSPORT_ERROR");
    }

    printf("  PASS testDefaultWindowsTransportReportsUnsupportedUrl\n");
}

void testDefaultWindowsTransportRejectsPlainHttp() {
	auto transport = keystar::createDefaultTransport();
	const auto publicHttp = transport->send({.url = "http://example.com/"});
	const auto loopbackWithoutOptIn = transport->send({.url = "http://127.0.0.1:8080/"});
	if (publicHttp.status_code != -1 || loopbackWithoutOptIn.status_code != -1) {
		throw std::runtime_error("default Windows transport accepted plaintext HTTP");
	}
	printf("  PASS testDefaultWindowsTransportRejectsPlainHttp\n");
}
#endif

}  // namespace

void run_transport_tests() {
    for (const auto& url : {"http://127.0.0.1:80@attacker.example/", "http://localhost:abc/", "http://127.0.0.1:70000/", "https://user:pass@example.com/", "http://example.com/"}) {
        if (keystar::detail::allowedTransportURL(url, true)) throw std::runtime_error("unsafe transport URL accepted");
    }
    if (!keystar::detail::allowedTransportURL("https://api.example.com/v1/me", false)
        || !keystar::detail::allowedTransportURL("http://127.0.0.1:55231/", true)) throw std::runtime_error("valid transport URL rejected");
#ifdef _WIN32
    if (const char* url = std::getenv("KEYSTAR_TRANSPORT_TEST_URL")) {
        auto transport = keystar::createDefaultTransport();
        const auto response = transport->send({.url = url, .allow_insecure_loopback = true});
        if (response.status_code != 200) throw std::runtime_error("valid local WinHTTP request failed");
    }
#endif
    printf("Running transport tests...\n");
    testFakeTransportRecordsRequests();
    testHttpResponseOk();
#ifdef _WIN32
    testDefaultWindowsTransportIsAvailable();
	testDefaultWindowsTransportReportsUnsupportedUrl();
	testDefaultWindowsTransportRejectsPlainHttp();
#endif
    printf("  All transport tests passed.\n");
}
