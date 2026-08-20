#pragma once

#include <string>

namespace keystar {

/// Error codes matching the backend error codes. The SDK never requires
/// string comparison; callers switch on the enum.
enum class ErrorCode {
    Ok = 0,
    InvalidCredentials,
    LicenseRequired,
    LicenseExpired,
    LicenseRevoked,
    DeviceLimitReached,
    DeviceRejected,
    DeviceVerificationFailed,
    ApplicationSuspended,
    Maintenance,
    RateLimited,
    NetworkError,
    ServerError,
    InvalidResponse,
    InvalidRequest,
    ChallengeExpired,
    ChallengeConsumed,
    RefreshTokenExpired,
    RefreshTokenRevoked,
    RefreshTokenReuse,
    TPMRequired,
    InsufficientScope,
    InvalidProduct,
};

/// Error carries a structured code plus a human-readable message.
class Error {
public:
    Error() : code_(ErrorCode::Ok) {}
    explicit Error(ErrorCode code, std::string message = {})
        : code_(code), message_(std::move(message)) {}

    ErrorCode code() const noexcept { return code_; }
    const std::string& message() const noexcept { return message_; }

    bool ok() const noexcept { return code_ == ErrorCode::Ok; }
    explicit operator bool() const noexcept { return ok(); }

private:
    ErrorCode code_;
    std::string message_;
};

/// Maps a backend JSON error code string to an ErrorCode.
ErrorCode mapErrorCode(const std::string& code);

}  // namespace keystar
