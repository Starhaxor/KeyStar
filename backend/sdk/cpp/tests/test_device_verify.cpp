#include "keystar/device_identity.hpp"

#include <cassert>
#include <cstdio>

namespace {

/// FakeDeviceProvider for testing.
class FakeDeviceProvider : public keystar::DeviceIdentityProvider {
public:
    keystar::DeviceIdentity collect() override {
        return {
            .smbios_uuid = "test-uuid",
            .motherboard_serial = "test-mobo",
            .bios_serial = "test-bios",
            .system_disk_serial = "test-disk",
            .machine_guid = "test-guid",
            .fingerprint = "test-fp",
            .tpm_public_key = "test-pubkey",
            .tpm_public_key_sha256 = "test-sha256",
        };
    }

    keystar::DeviceProof signChallenge(std::span<const uint8_t> challenge) override {
        // Return a deterministic "signature" based on the challenge size.
        return {
            .challenge_signature = "sig-" + std::to_string(challenge.size()),
            .device_public_key = "pubkey-from-challenge",
        };
    }

    bool hasTpm() const noexcept override { return hasTpm_; }
    bool hasTpm_ = true;
};

void testDeviceIdentityCollect() {
    FakeDeviceProvider provider;
    auto identity = provider.collect();

    assert(identity.smbios_uuid == "test-uuid");
    assert(identity.motherboard_serial == "test-mobo");
    assert(identity.bios_serial == "test-bios");
    assert(identity.system_disk_serial == "test-disk");
    assert(identity.machine_guid == "test-guid");
    assert(identity.fingerprint == "test-fp");

    printf("  PASS testDeviceIdentityCollect\n");
}

void testDeviceIdentitySignChallenge() {
    FakeDeviceProvider provider;
    uint8_t challenge[] = {0x01, 0x02, 0x03, 0x04};
    auto proof = provider.signChallenge(std::span<const uint8_t>(challenge, 4));

    assert(proof.challenge_signature == "sig-4");
    assert(proof.device_public_key == "pubkey-from-challenge");

    printf("  PASS testDeviceIdentitySignChallenge\n");
}

void testHasTpm() {
    FakeDeviceProvider provider;
    assert(provider.hasTpm());

    provider.hasTpm_ = false;
    assert(!provider.hasTpm());

    printf("  PASS testHasTpm\n");
}

}  // namespace

void run_device_verify_tests() {
    printf("Running device verify tests...\n");
    testDeviceIdentityCollect();
    testDeviceIdentitySignChallenge();
    testHasTpm();
    printf("  All device verify tests passed.\n");
}
