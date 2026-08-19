import tempfile
import unittest
import uuid
from pathlib import Path

from licensehub_licensing import LicenseClient, LicenseCoreError, LicenseState


class WrapperSmokeTest(unittest.TestCase):
    def test_abi_status_device_and_error(self):
        config = {
            "product_id": "wrapper_smoke",
            "server_url": "http://localhost:18080",
            "cache_dir": str(Path(tempfile.gettempdir()) / "licensehub-python-test" / uuid.uuid4().hex),
            "public_keys": {"test": "11qYAYdk9J2EORuRTvM9P4BKrMvBf7d7n8U8rTjU5YI="},
            "allow_insecure_localhost": True,
        }
        with LicenseClient.initialize(config) as client:
            self.assertEqual(client.status().state, LicenseState.NOT_ACTIVATED)
            self.assertTrue(client.device_id.startswith("dev_"))
            with self.assertRaises(LicenseCoreError) as caught: client.require_entitlement("pro")
            self.assertEqual(caught.exception.code, 41)


if __name__ == "__main__": unittest.main()
