"""Validate LicenseHub protocol schemas and endpoint-specific request drift."""

import json
from pathlib import Path

import yaml
from jsonschema import Draft202012Validator, ValidationError


ROOT = Path(__file__).resolve().parent
WORKSPACE = ROOT.parent


def load_json(relative: str):
    return json.loads((ROOT / relative).read_text(encoding="utf-8"))


request_schema = load_json("client-request.schema.json")
lease_schema = load_json("lease.schema.json")
openapi = yaml.safe_load((ROOT / "openapi.yaml").read_text(encoding="utf-8"))

Draft202012Validator.check_schema(request_schema)
Draft202012Validator.check_schema(lease_schema)

request_validator = Draft202012Validator(request_schema)
request_validator.validate(load_json("fixtures/activate-request.json"))
request_validator.validate(load_json("fixtures/lease-request.json"))

activate_ref = openapi["paths"]["/v1/client/activate"]["post"]["requestBody"]["content"]["application/json"]["schema"]["$ref"]
refresh_ref = openapi["paths"]["/v1/client/refresh"]["post"]["requestBody"]["content"]["application/json"]["schema"]["$ref"]
deactivate_ref = openapi["paths"]["/v1/client/deactivate"]["post"]["requestBody"]["content"]["application/json"]["schema"]["$ref"]
assert activate_ref.endswith("/ActivateRequest")
assert refresh_ref.endswith("/LeaseRequest")
assert deactivate_ref.endswith("/LeaseRequest")

components = openapi["components"]["schemas"]
Draft202012Validator(components["ActivateRequest"]).validate(load_json("fixtures/activate-request.json"))
lease_request_schema = dict(components["LeaseRequest"])
lease_request_schema["properties"] = dict(lease_request_schema["properties"])
lease_request_schema["properties"]["lease"] = components["Lease"]
lease_request_validator = Draft202012Validator(lease_request_schema)
lease_request_validator.validate(load_json("fixtures/lease-request.json"))
try:
    lease_request_validator.validate(load_json("fixtures/refresh-with-license-key.json"))
except ValidationError:
    pass
else:
    raise AssertionError("refresh/deactivate must reject license_key in place of lease")

Draft202012Validator(components["PublicKeysResponse"]).validate(load_json("fixtures/public-keys-response.json"))
assert "/v1/client/public-keys" in openapi["paths"]

# Source-to-contract drift guard: server and universal core must agree on which
# credential is sent at each stage.
handler_source = (WORKSPACE / "server/internal/handler/license.go").read_text(encoding="utf-8")
transport_source = (WORKSPACE / "core/license-core/src/transport.rs").read_text(encoding="utf-8")
main_source = (WORKSPACE / "server/cmd/server/main.go").read_text(encoding="utf-8")

for expected in (
    'LicenseKey string `json:"license_key" binding:"required"`',
    'Lease     string `json:"lease" binding:"required"`',
    "req, ok := bindClientLeaseRequest(c)",
):
    assert expected in handler_source, f"server contract drift: missing {expected}"

for expected in (
    "pub license_key: &'a str",
    "pub lease: &'a str",
    'self.post("/v1/client/activate", &request)',
    'self.post("/v1/client/refresh", &request)',
    'self.post("/v1/client/deactivate", &request)',
):
    assert expected in transport_source, f"core contract drift: missing {expected}"

assert 'client.GET("/public-keys"' in main_source
for expected in (
    "allLicenseKeys := licenseSvc.SigningPublicKeys()",
    "licensePublicKeyMap[kid] = pubBase64",
    'c.JSON(http.StatusOK, gin.H{"keys": licensePublicKeyMap})',
):
    assert expected in main_source, f"server key-ring drift: missing {expected}"

print("PROTOCOL_CONTRACT_OK")
