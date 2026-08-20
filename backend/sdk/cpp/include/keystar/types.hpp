#pragma once

#include <string>
#include <vector>

namespace keystar {

/// Options for constructing a Client. All fields are required except
/// base_url which defaults to the production endpoint.
struct ClientOptions {
    std::string application_id;
    std::string publishable_key;
    std::string base_url = "https://api.keystar.dev";
};

/// Login result on success.
struct UserProfile {
    std::string id;
    std::string email;
    std::string status;
    std::string created_at;
};

/// The result of a login or verify operation.
struct SessionResult {
    std::string access_token;
    std::string refresh_token;
    std::string token_expires_at;
    std::string license_id;
    std::string device_id;
    UserProfile user;
};

/// Device identity signals collected from the hardware.
struct DeviceIdentity {
    std::string smbios_uuid;
    std::string motherboard_serial;
    std::string bios_serial;
    std::string system_disk_serial;
    std::string machine_guid;
    std::string fingerprint;
    std::string tpm_public_key;      // base64-encoded TPM public key
    std::string tpm_public_key_sha256;
};

/// The result of signing a challenge with the device key.
struct DeviceProof {
    std::string challenge_signature;  // base64-encoded signature
    std::string device_public_key;    // base64-encoded public key
};

/// Feature flags returned with the session.
struct Features {
    std::vector<std::string> flags;
};

/// License information returned by /v1/me.
struct LicenseInfo {
    std::string id;
    std::string product;
    std::string status;
    int level;
    int max_devices;
    std::string expires_at;
};

/// Server response envelope for successful JSON responses.
template <typename T>
struct ApiResponse {
    bool ok = false;
    T data;
    std::string error_code;
    std::string error_message;

    bool has_error() const noexcept { return !error_code.empty(); }
};

}  // namespace keystar
