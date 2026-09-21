#pragma once
#include <string>
#include <vector>

bool is_directory(const std::string& path);
std::vector<std::string> list_files_recursive(const std::string& root);
