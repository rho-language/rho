add_executable(rho
    main.cpp
    driver.cpp
    ...
)

target_include_directories(rho PRIVATE ${CMAKE_CURRENT_SOURCE_DIR})
target_compile_features(rho PRIVATE cxx_std_17)

# instalăm binarul în build tree
set_target_properties(rho PROPERTIES
    RUNTIME_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}
)
