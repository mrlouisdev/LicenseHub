#ifndef LICENSE_CORE_H
#define LICENSE_CORE_H

#include <stddef.h>
#include <stdint.h>

#ifdef _WIN32
#  ifdef LICENSE_CORE_BUILD
#    define LICENSE_API __declspec(dllexport)
#  else
#    define LICENSE_API __declspec(dllimport)
#  endif
#else
#  define LICENSE_API
#endif

#ifdef __cplusplus
extern "C" {
#endif

#define LICENSE_CORE_ABI_VERSION 1u
typedef uint64_t license_handle_t;

LICENSE_API uint32_t license_abi_version(void);
LICENSE_API int32_t license_initialize(const char *config_json, license_handle_t *out_handle);
LICENSE_API int32_t license_shutdown(license_handle_t handle);
LICENSE_API int32_t license_activate(license_handle_t handle, const char *license_key);
LICENSE_API ptrdiff_t license_status(license_handle_t handle, char *buffer, size_t buffer_len);
LICENSE_API ptrdiff_t license_check(license_handle_t handle, char *buffer, size_t buffer_len);
LICENSE_API int32_t license_require_entitlement(license_handle_t handle, const char *entitlement);
LICENSE_API int32_t license_refresh(license_handle_t handle);
LICENSE_API int32_t license_deactivate(license_handle_t handle);
LICENSE_API ptrdiff_t license_device_id(license_handle_t handle, char *buffer, size_t buffer_len);
LICENSE_API ptrdiff_t license_last_error(char *buffer, size_t buffer_len);

#ifdef __cplusplus
}
#endif
#endif

