"use strict";
const assert = require("node:assert/strict");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");
const { LicenseClient, LicenseCoreError } = require("../src");

test("ABI, status, device id and native error surface", () => {
  const client = LicenseClient.initialize({
    product_id: "wrapper_smoke",
    server_url: "http://localhost:18080",
    cache_dir: path.join(os.tmpdir(), "licensehub-node-test", `${process.pid}-${Date.now()}`),
    public_keys: { test: "11qYAYdk9J2EORuRTvM9P4BKrMvBf7d7n8U8rTjU5YI=" },
    allow_insecure_localhost: true,
  });
  try {
    assert.equal(client.status().state, "not_activated");
    assert.match(client.deviceId, /^dev_/);
    assert.throws(() => client.requireEntitlement("pro"), error => error instanceof LicenseCoreError && error.code === 41);
  } finally { client.close(); }
});
