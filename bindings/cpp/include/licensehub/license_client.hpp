#pragma once

#include "license_core.h"

#include <cstdint>
#include <map>
#include <stdexcept>
#include <string>
#include <utility>
#include <vector>

namespace licensehub {

class error final : public std::runtime_error {
public:
    error(int code, const std::string& message)
        : std::runtime_error("License core error " + std::to_string(code) + ": " + message), code_(code) {}
    [[nodiscard]] int code() const noexcept { return code_; }
private:
    int code_;
};

struct config {
    std::string product_id;
    std::string server_url;
    std::string cache_dir;
    std::map<std::string, std::string> public_keys;
    std::int64_t clock_rollback_tolerance_seconds = 300;
    std::uint64_t request_timeout_seconds = 15;
    bool allow_insecure_localhost = false;

    [[nodiscard]] std::string to_json() const {
        auto quote = [](const std::string& value) {
            std::string result = "\"";
            for (const unsigned char c : value) {
                switch (c) {
                    case '\\': result += "\\\\"; break;
                    case '"': result += "\\\""; break;
                    case '\b': result += "\\b"; break;
                    case '\f': result += "\\f"; break;
                    case '\n': result += "\\n"; break;
                    case '\r': result += "\\r"; break;
                    case '\t': result += "\\t"; break;
                    default:
                        if (c < 0x20) throw std::invalid_argument("configuration contains a control character");
                        result += static_cast<char>(c);
                }
            }
            return result + "\"";
        };
        std::string keys = "{";
        bool first = true;
        for (const auto& [kid, value] : public_keys) {
            if (!first) keys += ',';
            first = false;
            keys += quote(kid) + ':' + quote(value);
        }
        keys += '}';
        return "{\"product_id\":" + quote(product_id) +
            ",\"server_url\":" + quote(server_url) +
            ",\"cache_dir\":" + quote(cache_dir) +
            ",\"public_keys\":" + keys +
            ",\"clock_rollback_tolerance_seconds\":" + std::to_string(clock_rollback_tolerance_seconds) +
            ",\"request_timeout_seconds\":" + std::to_string(request_timeout_seconds) +
            ",\"allow_insecure_localhost\":" + (allow_insecure_localhost ? "true" : "false") + '}';
    }
};

class client final {
public:
    explicit client(const config& value) {
        if (license_abi_version() != LICENSE_CORE_ABI_VERSION) throw std::runtime_error("unsupported License Core ABI");
        const auto json = value.to_json();
        check(license_initialize(json.c_str(), &handle_));
    }
    ~client() { if (handle_ != 0) (void)license_shutdown(handle_); }
    client(const client&) = delete;
    client& operator=(const client&) = delete;
    client(client&& other) noexcept : handle_(std::exchange(other.handle_, 0)) {}
    client& operator=(client&& other) noexcept {
        if (this != &other) {
            if (handle_ != 0) (void)license_shutdown(handle_);
            handle_ = std::exchange(other.handle_, 0);
        }
        return *this;
    }

    void activate(const std::string& value) { check(license_activate(handle_, value.c_str())); }
    void refresh() { check(license_refresh(handle_)); }
    void deactivate() { check(license_deactivate(handle_)); }
    void require_entitlement(const std::string& value) { check(license_require_entitlement(handle_, value.c_str())); }
    [[nodiscard]] std::string status_json() const { return read(license_status); }
    [[nodiscard]] std::string device_id() const { return read(license_device_id); }

private:
    using string_function = ptrdiff_t (*)(license_handle_t, char*, size_t);
    license_handle_t handle_ = 0;

    static std::string last_error() {
        const auto required = license_last_error(nullptr, 0);
        if (required <= 1) return "unknown native error";
        std::vector<char> output(static_cast<std::size_t>(required));
        if (license_last_error(output.data(), output.size()) < 0) return "unknown native error";
        return {output.data()};
    }
    static void check(std::int32_t result) {
        if (result < 0) throw error(-result, last_error());
    }
    [[nodiscard]] std::string read(string_function function) const {
        const auto required = function(handle_, nullptr, 0);
        if (required < 0) throw error(static_cast<int>(-required), last_error());
        std::vector<char> output(static_cast<std::size_t>(required));
        const auto written = function(handle_, output.data(), output.size());
        if (written < 0) throw error(static_cast<int>(-written), last_error());
        return {output.data()};
    }
};

} // namespace licensehub
