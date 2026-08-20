#ifdef _WIN32

#include "keystar/transport.hpp"

// WinHTTP-based HTTP transport for Windows.
// This is a stub; the full implementation would use:
//   - WinHttpOpen
//   - WinHttpConnect
//   - WinHttpOpenRequest
//   - WinHttpSendRequest
//   - WinHttpReceiveResponse
//
// For now, this file provides the factory function that returns nullptr
// (falling back to libcurl on Windows if available, or the caller must
// supply a transport).

namespace keystar {

std::shared_ptr<Transport> createDefaultTransport() {
    // TODO: Implement WinHTTP transport.
    // 1. WinHttpOpen(L"KeyStarSDK/1.0", ...)
    // 2. WinHttpConnect(session, host, port, ...)
    // 3. WinHttpOpenRequest(connect, method, path, ...)
    // 4. WinHttpSendRequest(request, headers, ...)
    // 5. WinHttpReceiveResponse(request, ...)
    // 6. WinHttpReadData(request, buffer, ...)
    //
    // Return nullptr to signal that the caller must provide a transport.
    return nullptr;
}

}  // namespace keystar

#endif  // _WIN32
