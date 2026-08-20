#include "keystar/error.hpp"

#include <cassert>
#include <cstdio>

namespace {

void testErrorDefault() {
    keystar::Error err;
    assert(err.ok());
    assert(err.code() == keystar::ErrorCode::Ok);
    assert(err.message().empty());
    printf("  PASS testErrorDefault\n");
}

void testErrorWithCode() {
    keystar::Error err(keystar::ErrorCode::LicenseExpired, "license expired");
    assert(!err.ok());
    assert(err.code() == keystar::ErrorCode::LicenseExpired);
    assert(err.message() == "license expired");
    assert(static_cast<bool>(err) == false);
    printf("  PASS testErrorWithCode\n");
}

void testMapErrorCode() {
    assert(keystar::mapErrorCode("INVALID_CREDENTIALS") == keystar::ErrorCode::InvalidCredentials);
    assert(keystar::mapErrorCode("LICENSE_EXPIRED") == keystar::ErrorCode::LicenseExpired);
    assert(keystar::mapErrorCode("DEVICE_LIMIT_REACHED") == keystar::ErrorCode::DeviceLimitReached);
    assert(keystar::mapErrorCode("CHALLENGE_EXPIRED") == keystar::ErrorCode::ChallengeExpired);
    assert(keystar::mapErrorCode("REFRESH_TOKEN_REUSE") == keystar::ErrorCode::RefreshTokenReuse);
    assert(keystar::mapErrorCode("TPM_REQUIRED") == keystar::ErrorCode::TPMRequired);
    assert(keystar::mapErrorCode("UNKNOWN_CODE") == keystar::ErrorCode::ServerError);
    printf("  PASS testMapErrorCode\n");
}

}  // namespace

void run_error_tests() {
    printf("Running error tests...\n");
    testErrorDefault();
    testErrorWithCode();
    testMapErrorCode();
    printf("  All error tests passed.\n");
}
