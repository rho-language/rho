# ------------------------------------------------------------------------------
# Rho CLI - Enterprise v3
# ------------------------------------------------------------------------------
# Builds the `rho` command-line tool:
#   rho run <file.rho>
#   rho build <file.rho> [-o out.c]
#
# This tool depends on the Rho compiler orchestrator (src/) and uses Go.
# ------------------------------------------------------------------------------

cmake_minimum_required(VERSION 3.16)

project(rho-cli LANGUAGES NONE)

# ------------------------------------------------------------------------------
# Locate Go
# ------------------------------------------------------------------------------
find_package(Go REQUIRED)

# ------------------------------------------------------------------------------
# Build rho CLI
# ------------------------------------------------------------------------------
add_custom_target(rho
    COMMAND ${GO_EXECUTABLE} build -o ${CMAKE_BINARY_DIR}/rho ./cmd/rho
    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
    COMMENT "Building Rho CLI"
)

# ------------------------------------------------------------------------------
# Install rho binary into build tree (so rho-test can call it)
# ------------------------------------------------------------------------------
set_target_properties(rho PROPERTIES
    RUNTIME_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}
)
