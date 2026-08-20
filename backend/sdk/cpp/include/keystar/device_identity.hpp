#pragma once

#include <cstdint>
#include <span>
#include <vector>

#include "types.hpp"

namespace keystar {

/// DeviceIdentityProvider is the abstract interface for collecting hardware
/// identity signals and signing challenges. Implementations are platform-
/// specific (Windows CNG/TPM, macOS, Linux).
class DeviceIdentityProvider {
public:
    virtual ~DeviceIdentityProvider() = default;

    /// Collect hardware identity signals (SMBIOS UUID, motherboard serial,
    /// BIOS serial, disk serial, machine GUID, fingerprint).
    virtual DeviceIdentity collect() = 0;

    /// Sign a challenge with the device key (TPM or software fallback).
    /// Returns the base64-encoded signature and public key.
    virtual DeviceProof signChallenge(std::span<const uint8_t> challenge) = 0;

    /// Returns true if a TPM is available and being used.
    virtual bool hasTpm() const noexcept = 0;
};

}  // namespace keystar
