// KeyStar C++ SDK — Test Runner
//
// Run with: ./keystar_tests
// Or via CTest: ctest --test-dir build

#include <cstdio>
#include <cstdlib>
#include <exception>

// Forward declarations of test suites.
void run_transport_tests();
void run_error_tests();
void run_json_parser_tests();
void run_auth_tests();
void run_device_verify_tests();
void run_token_store_tests();

int main() {
    setvbuf(stdout, nullptr, _IONBF, 0);
    printf("=== KeyStar C++ SDK Tests ===\n\n");

    int failures = 0;

    auto run = [&](const char* name, void (*fn)()) {
        printf("[%s]\n", name);
        try {
            fn();
        } catch (const std::exception& e) {
            printf("  FATAL: %s\n", e.what());
            failures++;
        } catch (...) {
            printf("  FATAL: unknown exception\n");
            failures++;
        }
        printf("\n");
    };

    run("Transport", run_transport_tests);
    run("Error", run_error_tests);
    run("JSON Parser", run_json_parser_tests);
    run("Auth", run_auth_tests);
    run("Device Verify", run_device_verify_tests);
    run("Token Store", run_token_store_tests);

    if (failures > 0) {
        printf("FAILED: %d test suite(s) had errors\n", failures);
        return 1;
    }

    printf("=== All tests passed ===\n");
    return 0;
}
