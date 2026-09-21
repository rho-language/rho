#include "fs.hpp"
#include <filesystem>

bool is_directory(const std::string& path) {
    return std::filesystem::is_directory(path);
}

std::vector<std::string> list_files_recursive(const std::string& root) {
    std::vector<std::string> out;
    for (auto& p : std::filesystem::recursive_directory_iterator(root)) {
        if (p.is_regular_file()) {
            out.push_back(p.path().string());
        }
    }
    return out;
}
