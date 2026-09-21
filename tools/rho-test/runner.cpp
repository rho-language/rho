#include "runner.hpp"
#include "process.hpp"
#include "util.hpp"
#include <filesystem>

static std::string rho_bin = "rho";      // compiler CLI
static std::string cc_bin  = "gcc";      // sau clang

TestResult run_test_file(const std::string& file) {
    TestResult res{file, false};

    // 1. build Rho → C
    std::string c_out = file + ".c";
    int c_code = run_process(rho_bin, {"build", file, "-o", c_out});
    if (c_code != 0) {
        print_fail("[BUILD FAIL] " + file);
        return res;
    }

    // 2. compile C → exe
    std::string exe = file + ".exe";
    int cc_code = run_process(cc_bin, {c_out, "-o", exe});
    if (cc_code != 0) {
        print_fail("[CC FAIL] " + file);
        return res;
    }

    // 3. run exe
    int run_code = run_process(exe, {});
    if (run_code == 0) {
        print_pass("[PASS] " + file);
        res.passed = true;
    } else {
        print_fail("[FAIL] " + file);
    }

    // optional: cleanup c_out/exe later
    return res;
}
