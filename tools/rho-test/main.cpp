#include "discover.hpp"
#include "runner.hpp"
#include "util.hpp"
#include <vector>
#include <string>
#include <iostream>

int main(int argc, char** argv) {
    if (argc < 2) {
        print_info("Usage: rho-test <file.rho | dir>");
        return 1;
    }

    std::string target = argv[1];
    auto tests = discover_tests(target);

    if (tests.empty()) {
        print_info("No tests found.");
        return 1;
    }

    int passed = 0;
    int failed = 0;

    for (auto& f : tests) {
        print_info("[RUN] " + f);
        auto res = run_test_file(f);
        if (res.passed) passed++;
        else failed++;
    }

    std::cout << "\n" << tests.size() << " tests, "
              << passed << " passed, "
              << failed << " failed\n";

    return failed == 0 ? 0 : 1;
}
