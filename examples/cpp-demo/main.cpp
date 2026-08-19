#include <licensehub/license_client.hpp>
#include <cstdlib>
#include <iostream>

int main(int argc, char** argv) {
    if (argc < 5) {
        std::cerr << "usage: demo <product> <server-url> <kid> <public-key> [activation-value]\n";
        return 2;
    }
    const std::string endpoint = argv[2];
    const bool local_http = endpoint.rfind("http://localhost", 0) == 0 || endpoint.rfind("http://127.0.0.1", 0) == 0;
    licensehub::config config{argv[1], endpoint, "license-cache", {{argv[3], argv[4]}}, 300, 15, local_http};
    licensehub::client client(config);
    if (argc > 5) client.activate(argv[5]);
    std::cout << client.status_json() << '\n';
    return 0;
}
