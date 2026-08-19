#include <licensehub/license_client.hpp>
#include <cstdlib>
#include <filesystem>
#include <iostream>

int main() {
    auto cache = std::filesystem::temp_directory_path() / "licensehub-cpp-smoke";
    licensehub::config config{
        "wrapper_smoke", "http://localhost:18080", cache.string(),
        {{"test", "11qYAYdk9J2EORuRTvM9P4BKrMvBf7d7n8U8rTjU5YI="}}, 300, 15, true
    };
    licensehub::client client(config);
    if (client.status_json().find("not_activated") == std::string::npos) return 1;
    if (client.device_id().rfind("dev_", 0) != 0) return 2;
    try { client.require_entitlement("pro"); return 3; }
    catch (const licensehub::error& caught) { if (caught.code() != 41) return 4; }
    std::cout << "PASS cpp wrapper ABI/status/error smoke test\n";
    return 0;
}
