add_executable(rho-test
    main.cpp
    discover.cpp
    runner.cpp
    process.cpp
    fs.cpp
)

target_include_directories(rho-test PRIVATE ${CMAKE_CURRENT_SOURCE_DIR})
target_compile_features(rho-test PRIVATE cxx_std_17)
