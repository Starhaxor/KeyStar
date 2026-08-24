#include "keystar/client.hpp"

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
#endif

}  // namespace

void run_transport_tests() {
    printf("Running transport tests...\n");
    testFakeTransportRecordsRequests();
    testHttpResponseOk();
#ifdef _WIN32
    testDefaultWindowsTransportIsAvailable();
#endif
    printf("  All transport tests passed.\n");
}
