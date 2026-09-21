#pragma once
#include <string>
#include <iostream>

inline void print_pass(const std::string& msg) {
    std::cout << "\x1b[32m" << msg << "\x1b[0m\n";
}

inline void print_fail(const std::string& msg) {
    std::cout << "\x1b[31m" << msg << "\x1b[0m\n";
}

inline void print_info(const std::string& msg) {
    std::cout << msg << "\n";
}
