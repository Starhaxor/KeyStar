#include "keystar/error.hpp"

#include <unordered_map>

namespace keystar {

ErrorCode mapErrorCode(const std::string& code) {
    static const std::unordered_map<std::string, ErrorCode> table = {
        {"INVALID_CREDENTIALS",       ErrorCode::InvalidCredentials},
        {"INVALID_CREDENTIAL",        ErrorCode::InvalidCredentials},
        {"LICENSE_REQUIRED",          ErrorCode::LicenseRequired},
        {"LICENSE_EXPIRED",           ErrorCode::LicenseExpired},
        {"LICENSE_REVOKED",           ErrorCode::LicenseRevoked},
        {"DEVICE_LIMIT_REACHED",      ErrorCode::DeviceLimitReached},
        {"DEVICE_REVOKED",            ErrorCode::DeviceRejected},
        {"INVALID_DEVICE_SIGNATURE",  ErrorCode::DeviceVerificationFailed},
        {"APPLICATION_SUSPENDED",     ErrorCode::ApplicationSuspended},
        {"MAINTENANCE",               ErrorCode::Maintenance},
        {"RATE_LIMITED",              ErrorCode::RateLimited},
        {"CHALLENGE_EXPIRED",         ErrorCode::ChallengeExpired},
        {"CHALLENGE_CONSUMED",        ErrorCode::ChallengeConsumed},
        {"REFRESH_TOKEN_EXPIRED",     ErrorCode::RefreshTokenExpired},
        {"REFRESH_TOKEN_REVOKED",     ErrorCode::RefreshTokenRevoked},
        {"REFRESH_TOKEN_REUSE",       ErrorCode::RefreshTokenReuse},
        {"INVALID_REFRESH_TOKEN",     ErrorCode::RefreshTokenExpired},
        {"TPM_REQUIRED",              ErrorCode::TPMRequired},
        {"INSUFFICIENT_SCOPE",        ErrorCode::InsufficientScope},
        {"INVALID_PRODUCT",           ErrorCode::InvalidProduct},
        {"USER_NOT_FOUND",            ErrorCode::InvalidCredentials},
        {"INVALID_REQUEST",           ErrorCode::InvalidRequest},
        {"SERVER_ERROR",              ErrorCode::ServerError},
    };

    auto it = table.find(code);
    if (it != table.end()) {
        return it->second;
    }
    return ErrorCode::ServerError;
}

}  // namespace keystar
