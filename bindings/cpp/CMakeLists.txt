cmake_minimum_required(VERSION 3.20)
project(LicenseHubCpp LANGUAGES CXX)

set(LICENSE_CORE_ROOT "${CMAKE_CURRENT_LIST_DIR}/../../core" CACHE PATH "License Core build root")
add_library(licensehub_core SHARED IMPORTED GLOBAL)
if(WIN32)
  set_target_properties(licensehub_core PROPERTIES
    IMPORTED_LOCATION "${LICENSE_CORE_ROOT}/target/release/license_core.dll"
    IMPORTED_IMPLIB "${LICENSE_CORE_ROOT}/target/release/license_core.dll.lib")
else()
  set_target_properties(licensehub_core PROPERTIES IMPORTED_LOCATION "${LICENSE_CORE_ROOT}/target/release/liblicense_core.so")
endif()

add_library(licensehub_cpp INTERFACE)
target_compile_features(licensehub_cpp INTERFACE cxx_std_17)
target_include_directories(licensehub_cpp INTERFACE "${CMAKE_CURRENT_LIST_DIR}/include" "${LICENSE_CORE_ROOT}/include")
target_link_libraries(licensehub_cpp INTERFACE licensehub_core)

function(licensehub_copy_runtime target)
  if(WIN32)
    add_custom_command(TARGET ${target} POST_BUILD COMMAND ${CMAKE_COMMAND} -E copy_if_different
      "${LICENSE_CORE_ROOT}/target/release/license_core.dll" "$<TARGET_FILE_DIR:${target}>")
  endif()
endfunction()

option(LICENSEHUB_BUILD_TESTS "Build C++ smoke test" OFF)
if(LICENSEHUB_BUILD_TESTS)
  add_executable(licensehub_cpp_smoke tests/smoke.cpp)
  target_link_libraries(licensehub_cpp_smoke PRIVATE licensehub_cpp)
  licensehub_copy_runtime(licensehub_cpp_smoke)
endif()
