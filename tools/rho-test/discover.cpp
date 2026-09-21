#include "discover.hpp"
#include "fs.hpp"
#include <filesystem>

std::vector<std::string> discover_tests(const std::string& path) {
    std::vector<std::string> out;

    if (!is_directory(path)) {
        // single file
        if (path.size() >= 10 && path.substr(path.size() - 10) == "_test.rho") {
            out.push_back(path);
        }
        return out;
    }

    auto files = list_files_recursive(path);
    for (auto& f : files) {
        if (f.size() >= 10 && f.substr(f.size() - 10) == "_test.rho") {
            out.push_back(f);
        }
    }

    return out;
}
