#ifdef _WIN32

#include "keystar/token_store.hpp"

#include <windows.h>
#include <wincrypt.h>

#include <array>
#include <cstdint>
#include <filesystem>
#include <fstream>
#include <memory>
#include <optional>
#include <string>
#include <vector>

namespace keystar {
namespace {

constexpr size_t kMaxStoredSessionBytes = 1024 * 1024;

void appendField(std::vector<BYTE>& bytes, const std::string& value) {
    const auto length = static_cast<std::uint32_t>(value.size());
    for (unsigned shift = 0; shift < 32; shift += 8) {
        bytes.push_back(static_cast<BYTE>((length >> shift) & 0xff));
    }
    bytes.insert(bytes.end(), value.begin(), value.end());
}

bool readField(const std::vector<BYTE>& bytes, size_t& offset, std::string& value) {
    if (bytes.size() - offset < sizeof(std::uint32_t)) return false;
    std::uint32_t length = 0;
    for (unsigned shift = 0; shift < 32; shift += 8) {
        length |= static_cast<std::uint32_t>(bytes[offset++]) << shift;
    }
    if (length > bytes.size() - offset) return false;
    value.assign(reinterpret_cast<const char*>(bytes.data() + offset), length);
    offset += length;
    return true;
}

std::vector<BYTE> serialize(const StoredSession& session) {
	std::vector<BYTE> bytes;
	const size_t payloadSize = session.refresh_token.size() + session.user_id.size() + session.device_id.size() +
				  session.license_id.size() + session.expires_at.size() + 5 * sizeof(std::uint32_t);
	if (payloadSize > kMaxStoredSessionBytes) return bytes;
	bytes.reserve(payloadSize);
    appendField(bytes, session.refresh_token);
    appendField(bytes, session.user_id);
    appendField(bytes, session.device_id);
    appendField(bytes, session.license_id);
    appendField(bytes, session.expires_at);
    return bytes;
}

std::optional<StoredSession> deserialize(const std::vector<BYTE>& bytes) {
    StoredSession session;
    size_t offset = 0;
    if (!readField(bytes, offset, session.refresh_token) || !readField(bytes, offset, session.user_id) ||
        !readField(bytes, offset, session.device_id) || !readField(bytes, offset, session.license_id) ||
        !readField(bytes, offset, session.expires_at) || offset != bytes.size()) {
        return std::nullopt;
    }
    return session;
}

std::optional<std::wstring> localAppDataPath() {
    DWORD length = GetEnvironmentVariableW(L"LOCALAPPDATA", nullptr, 0);
    if (length == 0) return std::nullopt;
    std::wstring value(length, L'\0');
    const DWORD written = GetEnvironmentVariableW(L"LOCALAPPDATA", value.data(), length);
    if (written != length - 1) {
        return std::nullopt;
    }
    value.resize(written);
    return value;
}

std::optional<std::string> sha256Hex(const std::string& value) {
    HCRYPTPROV provider = 0;
    HCRYPTHASH hash = 0;
    if (!CryptAcquireContextW(&provider, nullptr, nullptr, PROV_RSA_AES, CRYPT_VERIFYCONTEXT) ||
        !CryptCreateHash(provider, CALG_SHA_256, 0, 0, &hash) ||
        !CryptHashData(hash, reinterpret_cast<const BYTE*>(value.data()),
                       static_cast<DWORD>(value.size()), 0)) {
        if (hash) CryptDestroyHash(hash);
        if (provider) CryptReleaseContext(provider, 0);
        return std::nullopt;
    }

    std::array<BYTE, 32> digest{};
    DWORD length = static_cast<DWORD>(digest.size());
    const bool ok = CryptGetHashParam(hash, HP_HASHVAL, digest.data(), &length, 0);
    CryptDestroyHash(hash);
    CryptReleaseContext(provider, 0);
    if (!ok || length != digest.size()) return std::nullopt;

    static constexpr char hex[] = "0123456789abcdef";
    std::string encoded;
    encoded.reserve(digest.size() * 2);
    for (BYTE byte : digest) {
        encoded.push_back(hex[byte >> 4]);
        encoded.push_back(hex[byte & 0x0f]);
    }
    return encoded;
}

std::optional<std::filesystem::path> recordPath(const std::string& applicationId) {
    const auto localAppData = localAppDataPath();
    const auto hash = sha256Hex(applicationId);
    if (!localAppData || !hash) return std::nullopt;
    return std::filesystem::path(*localAppData) / L"KeyStar" /
           std::filesystem::path(*hash + ".bin");
}

class DpapiTokenStore final : public TokenStore {
public:
    explicit DpapiTokenStore(std::string applicationId) : path_(recordPath(applicationId)) {}

    bool save(const StoredSession& session) override {
        if (!path_) return false;
		std::vector<BYTE> plaintext = serialize(session);
		if (plaintext.empty() || plaintext.size() > kMaxStoredSessionBytes) return false;
        DATA_BLOB input{static_cast<DWORD>(plaintext.size()), plaintext.data()};
        DATA_BLOB encrypted{};
        if (!CryptProtectData(&input, L"KeyStar session", nullptr, nullptr, nullptr,
                              CRYPTPROTECT_UI_FORBIDDEN, &encrypted)) {
            return false;
        }

        const std::filesystem::path directory = path_->parent_path();
        std::error_code error;
        std::filesystem::create_directories(directory, error);
        if (error) {
            LocalFree(encrypted.pbData);
            return false;
        }

        std::filesystem::path temporary = *path_;
        temporary += L"." + std::to_wstring(GetCurrentProcessId()) + L"." +
                     std::to_wstring(GetTickCount64()) + L".tmp";
        bool written = false;
        {
            std::ofstream output(temporary, std::ios::binary | std::ios::trunc);
            if (output) {
                output.write(reinterpret_cast<const char*>(encrypted.pbData), encrypted.cbData);
                output.flush();
                written = output.good();
            }
        }
        LocalFree(encrypted.pbData);
        if (!written) {
            std::filesystem::remove(temporary, error);
            return false;
        }

        if (!MoveFileExW(temporary.c_str(), path_->c_str(),
                         MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH)) {
            std::filesystem::remove(temporary, error);
            return false;
        }
        return true;
    }

    std::optional<StoredSession> load() override {
        if (!path_) return std::nullopt;
        std::error_code error;
        if (!std::filesystem::exists(*path_, error) || error) return std::nullopt;

        std::ifstream input(*path_, std::ios::binary | std::ios::ate);
        if (!input) return std::nullopt;
        const std::streamsize size = input.tellg();
		if (size <= 0 || size > static_cast<std::streamsize>(kMaxStoredSessionBytes)) return std::nullopt;
        input.seekg(0);
        std::vector<BYTE> encrypted(static_cast<size_t>(size));
        if (!encrypted.empty()) {
            input.read(reinterpret_cast<char*>(encrypted.data()), size);
            if (!input) return std::nullopt;
        }

        DATA_BLOB encryptedBlob{static_cast<DWORD>(encrypted.size()), encrypted.data()};
        DATA_BLOB plaintext{};
        if (!CryptUnprotectData(&encryptedBlob, nullptr, nullptr, nullptr, nullptr,
                                CRYPTPROTECT_UI_FORBIDDEN, &plaintext)) {
            return std::nullopt;
        }
		std::vector<BYTE> bytes(plaintext.pbData, plaintext.pbData + plaintext.cbData);
		LocalFree(plaintext.pbData);
		if (bytes.size() > kMaxStoredSessionBytes) return std::nullopt;
        return deserialize(bytes);
    }

    bool clear() override {
        if (!path_) return false;
        std::error_code error;
        const bool removed = std::filesystem::remove(*path_, error);
        return !error && (removed || !std::filesystem::exists(*path_, error));
    }

private:
    std::optional<std::filesystem::path> path_;
};

}  // namespace

std::shared_ptr<TokenStore> createPlatformTokenStore(const std::string& appId) {
    return std::make_shared<DpapiTokenStore>(appId);
}

}  // namespace keystar

#endif  // _WIN32
