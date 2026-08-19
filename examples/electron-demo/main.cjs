"use strict";
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { LicenseClient } = require("@licensehub/licensing");

const [manifestPath, activationValue] = process.argv.slice(2);
if (!manifestPath) throw new Error("usage: npm start -- <product.manifest.json> [activation-value]");
const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
const client = LicenseClient.initialize({ ...manifest, cache_dir: path.join(os.homedir(), ".licensehub", manifest.product_id) });
try {
  const status = activationValue ? client.activate(activationValue) : client.status();
  console.log(JSON.stringify(status, null, 2));
} finally { client.close(); }
