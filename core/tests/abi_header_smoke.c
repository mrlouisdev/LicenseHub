#include "../include/license_core.h"

static int consume_api(void) {
    license_handle_t handle = 0;
    (void)handle;
    return LICENSE_CORE_ABI_VERSION == 1u ? 0 : 1;
}

int main(void) {
    return consume_api();
}

