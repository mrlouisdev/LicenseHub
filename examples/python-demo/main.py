import json
import os
import sys
from pathlib import Path

from licensehub_licensing import LicenseClient

if len(sys.argv) < 2:
    raise SystemExit("usage: python main.py <product.manifest.json> [activation-value]")
manifest = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
config = {
    **manifest,
    "cache_dir": str(Path(os.getenv("LOCALAPPDATA", Path.home())) / "LicenseHub" / manifest["product_id"]),
}
with LicenseClient.initialize(config) as client:
    status = client.activate(sys.argv[2]) if len(sys.argv) > 2 else client.status()
    print(json.dumps(status.__dict__, default=lambda value: value.value, indent=2))
