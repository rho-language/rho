#pragma once
#include <string>

struct TestResult {
    std::string file;
    bool passed;
};

TestResult run_test_file(const std::string& file);
