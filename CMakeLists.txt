cmake_minimum_required(VERSION 3.16)
project(rho LANGUAGES C CXX)

include(CTest)

find_program(GO_EXECUTABLE go)
if(GO_EXECUTABLE)
    add_custom_target(rho ALL
        COMMAND ${GO_EXECUTABLE} build ./...
        WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}
        COMMENT "Building Rho Go compiler and tools"
    )

    if(BUILD_TESTING)
        add_test(NAME rho-go-test
            COMMAND ${GO_EXECUTABLE} test ./...
            WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}
        )
    endif()
else()
    message(WARNING "Go was not found; the rho target is unavailable")
endif()
