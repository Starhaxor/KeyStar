#ifdef _WIN32

#include "keystar/transport.hpp"

#include <windows.h>
#include <winhttp.h>

#include <memory>
#include <string>
#include <vector>

namespace keystar {
namespace {

constexpr int kTransportErrorStatus = -1;
constexpr size_t kMaxResponseBytes = 1024 * 1024;
constexpr char kTransportErrorBody[] =
    R"({"code":"TRANSPORT_ERROR","message":"WinHTTP request failed"})";

std::wstring utf8ToWide(const std::string& value) {
    if (value.empty()) return {};

    const int size = MultiByteToWideChar(
        CP_UTF8, MB_ERR_INVALID_CHARS, value.data(), static_cast<int>(value.size()), nullptr, 0);
    if (size <= 0) return {};

    std::wstring result(static_cast<size_t>(size), L'\0');
    if (MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, value.data(),
                            static_cast<int>(value.size()), result.data(), size) != size) {
        return {};
    }
    return result;
}

std::string wideToUtf8(const std::wstring& value) {
    if (value.empty()) return {};

    const int size = WideCharToMultiByte(CP_UTF8, 0, value.data(), static_cast<int>(value.size()),
                                         nullptr, 0, nullptr, nullptr);
    if (size <= 0) return {};

    std::string result(static_cast<size_t>(size), '\0');
    if (WideCharToMultiByte(CP_UTF8, 0, value.data(), static_cast<int>(value.size()), result.data(),
                            size, nullptr, nullptr) != size) {
        return {};
    }
    return result;
}

const wchar_t* methodName(HttpMethod method) {
    switch (method) {
        case HttpMethod::Get: return L"GET";
        case HttpMethod::Post: return L"POST";
        case HttpMethod::Patch: return L"PATCH";
        case HttpMethod::Put: return L"PUT";
        case HttpMethod::Delete: return L"DELETE";
    }
    return L"GET";
}

HttpResponse transportError() {
    return {.status_code = kTransportErrorStatus, .body = kTransportErrorBody};
}

class InternetHandle {
public:
    explicit InternetHandle(HINTERNET handle = nullptr) : handle_(handle) {}
    ~InternetHandle() {
        if (handle_) WinHttpCloseHandle(handle_);
    }

    InternetHandle(const InternetHandle&) = delete;
    InternetHandle& operator=(const InternetHandle&) = delete;

    HINTERNET get() const { return handle_; }
    explicit operator bool() const { return handle_ != nullptr; }

private:
    HINTERNET handle_;
};

void populateHeaders(HINTERNET request, HttpResponse& response) {
    DWORD bytes = 0;
    WinHttpQueryHeaders(request, WINHTTP_QUERY_RAW_HEADERS_CRLF,
                        WINHTTP_HEADER_NAME_BY_INDEX, nullptr, &bytes,
                        WINHTTP_NO_HEADER_INDEX);
	if (GetLastError() != ERROR_INSUFFICIENT_BUFFER || bytes == 0 || bytes > 64 * 1024) return;

    std::vector<wchar_t> raw(bytes / sizeof(wchar_t));
    if (!WinHttpQueryHeaders(request, WINHTTP_QUERY_RAW_HEADERS_CRLF,
                             WINHTTP_HEADER_NAME_BY_INDEX, raw.data(), &bytes,
                             WINHTTP_NO_HEADER_INDEX)) {
        return;
    }

    std::wstring headers(raw.data());
    size_t lineStart = headers.find(L"\r\n");
    if (lineStart == std::wstring::npos) return;
    lineStart += 2;
    while (lineStart < headers.size()) {
        const size_t lineEnd = headers.find(L"\r\n", lineStart);
        if (lineEnd == std::wstring::npos || lineEnd == lineStart) break;
        const std::wstring line = headers.substr(lineStart, lineEnd - lineStart);
        const size_t separator = line.find(L':');
        if (separator != std::wstring::npos) {
            response.headers[wideToUtf8(line.substr(0, separator))] =
                wideToUtf8(line.substr(separator + 1));
        }
        lineStart = lineEnd + 2;
    }
}

class WinHttpTransport final : public Transport {
public:
    HttpResponse send(const HttpRequest& request) override {
        const std::wstring url = utf8ToWide(request.url);
        if (url.empty()) return transportError();

        std::vector<wchar_t> urlBuffer(url.begin(), url.end());
        urlBuffer.push_back(L'\0');
        URL_COMPONENTS components{};
        components.dwStructSize = sizeof(components);
        if (!WinHttpCrackUrl(urlBuffer.data(), 0, 0, &components)) return transportError();

        const std::wstring host(components.lpszHostName, components.dwHostNameLength);
        std::wstring path(components.lpszUrlPath, components.dwUrlPathLength);
        path.append(components.lpszExtraInfo, components.dwExtraInfoLength);
		if (host.empty() || components.dwUserNameLength != 0 || components.dwPasswordLength != 0) return transportError();
        if (path.empty()) path = L"/";

        const bool secure = components.nScheme == INTERNET_SCHEME_HTTPS;
		const bool loopback = _wcsicmp(host.c_str(), L"localhost") == 0 || host == L"127.0.0.1" || host == L"::1";
		if (!secure && (components.nScheme != INTERNET_SCHEME_HTTP || !request.allow_insecure_loopback || !loopback)) return transportError();

        InternetHandle session(WinHttpOpen(L"KeyStarSDK/1.0", WINHTTP_ACCESS_TYPE_DEFAULT_PROXY,
                                           WINHTTP_NO_PROXY_NAME, WINHTTP_NO_PROXY_BYPASS, 0));
		if (!session) return transportError();
		if (!WinHttpSetTimeouts(session.get(), 5000, 5000, 10000, 10000)) return transportError();
		DWORD secureProtocols = WINHTTP_FLAG_SECURE_PROTOCOL_TLS1_2;
#ifdef WINHTTP_FLAG_SECURE_PROTOCOL_TLS1_3
		secureProtocols |= WINHTTP_FLAG_SECURE_PROTOCOL_TLS1_3;
#endif
		if (!WinHttpSetOption(session.get(), WINHTTP_OPTION_SECURE_PROTOCOLS, &secureProtocols, sizeof(secureProtocols))) return transportError();
        InternetHandle connection(WinHttpConnect(session.get(), host.c_str(), components.nPort, 0));
        if (!connection) return transportError();
        InternetHandle handle(WinHttpOpenRequest(connection.get(), methodName(request.method), path.c_str(),
                                                  nullptr, WINHTTP_NO_REFERER,
                                                  WINHTTP_DEFAULT_ACCEPT_TYPES,
                                                  secure ? WINHTTP_FLAG_SECURE : 0));
		if (!handle) return transportError();
		DWORD disabledFeatures = WINHTTP_DISABLE_REDIRECTS;
		if (!WinHttpSetOption(handle.get(), WINHTTP_OPTION_DISABLE_FEATURE, &disabledFeatures, sizeof(disabledFeatures))) return transportError();

        std::wstring headers;
		for (const auto& [name, value] : request.headers) {
			if (name.find_first_of("\r\n") != std::string::npos || value.find_first_of("\r\n") != std::string::npos) return transportError();
            const std::wstring wideName = utf8ToWide(name);
            const std::wstring wideValue = utf8ToWide(value);
            if (wideName.empty() || (value.size() != 0 && wideValue.empty())) return transportError();
            headers.append(wideName).append(L": ").append(wideValue).append(L"\r\n");
        }

        if (request.body.size() > MAXDWORD || headers.size() > MAXDWORD) return transportError();
        if (!WinHttpSendRequest(handle.get(),
                                headers.empty() ? WINHTTP_NO_ADDITIONAL_HEADERS : headers.c_str(),
                                static_cast<DWORD>(headers.size()),
                                request.body.empty() ? WINHTTP_NO_REQUEST_DATA :
                                                       const_cast<char*>(request.body.data()),
                                static_cast<DWORD>(request.body.size()),
                                static_cast<DWORD>(request.body.size()), 0) ||
            !WinHttpReceiveResponse(handle.get(), nullptr)) {
            return transportError();
        }

        HttpResponse response;
        DWORD statusSize = sizeof(response.status_code);
        if (!WinHttpQueryHeaders(handle.get(), WINHTTP_QUERY_STATUS_CODE | WINHTTP_QUERY_FLAG_NUMBER,
                                 WINHTTP_HEADER_NAME_BY_INDEX, &response.status_code, &statusSize,
                                 WINHTTP_NO_HEADER_INDEX)) {
            return transportError();
        }
        populateHeaders(handle.get(), response);

        for (;;) {
            DWORD available = 0;
            if (!WinHttpQueryDataAvailable(handle.get(), &available)) return transportError();
            if (available == 0) break;

			if (available > kMaxResponseBytes - response.body.size()) return transportError();
			std::string chunk(available, '\0');
            DWORD read = 0;
            if (!WinHttpReadData(handle.get(), chunk.data(), available, &read)) return transportError();
            response.body.append(chunk.data(), read);
            if (read == 0) break;
        }

        return response;
    }
};

}  // namespace

std::shared_ptr<Transport> createDefaultTransport() {
    return std::make_shared<WinHttpTransport>();
}

}  // namespace keystar

#endif  // _WIN32
