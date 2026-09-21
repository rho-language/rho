#include "process.hpp"
#include <string>
#include <vector>
#include <sstream>
#include <cstdlib>

int run_process(const std::string& cmd, const std::vector<std::string>& args) {
    std::ostringstream oss;
    oss << cmd;
    for (auto& a : args) {
        oss << " " << a;
    }
    std::string full = oss.str();
    return std::system(full.c_str());
}
