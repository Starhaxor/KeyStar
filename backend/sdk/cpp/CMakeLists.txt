cmake_minimum_required(VERSION 3.16)
project(keystar VERSION 0.1.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

# ── Options ──────────────────────────────────────────────────────────────────
option(KEYSTAR_BUILD_TESTS "Build SDK tests" ON)

# ── Sources ──────────────────────────────────────────────────────────────────
set(KESTARCH_SOURCES
    src/client.cpp
    src/auth.cpp
    src/device_verify.cpp
    src/error.cpp
    src/json_parser.cpp
)

# Platform-specific sources
if(WIN32)
    list(APPEND KESTARCH_SOURCES
        src/platform/transport_winhttp.cpp
        src/platform/token_store_dpapi.cpp
    )
else()
    list(APPEND KESTARCH_SOURCES
        src/transport_curl.cpp
        src/platform/token_store_posix.cpp
    )
endif()

# ── Library ──────────────────────────────────────────────────────────────────
add_library(keystar STATIC ${KESTARCH_SOURCES})
target_include_directories(keystar PUBLIC include)

# Dependencies
if(WIN32)
    target_link_libraries(keystar PRIVATE winhttp crypt32 advapi32)
else()
    find_package(CURL REQUIRED)
    target_link_libraries(keystar PRIVATE CURL::libcurl)
    target_link_libraries(keystar PRIVATE pthread)
endif()

# ── Tests ────────────────────────────────────────────────────────────────────
if(KEYSTAR_BUILD_TESTS)
    enable_testing()
    add_executable(keystar_tests
        tests/main.cpp
        tests/test_transport.cpp
        tests/test_auth.cpp
        tests/test_device_verify.cpp
        tests/test_token_store.cpp
        tests/test_error.cpp
        tests/test_json_parser.cpp
    )
	target_link_libraries(keystar_tests PRIVATE keystar)
	# Keep assertions active in Release test binaries; this suite intentionally
	# uses assert for contract checks.
	if(MSVC)
		target_compile_options(keystar_tests PRIVATE /UNDEBUG)
	else()
		target_compile_options(keystar_tests PRIVATE -UNDEBUG)
	endif()
	add_test(NAME keystar_tests COMMAND keystar_tests)
endif()
