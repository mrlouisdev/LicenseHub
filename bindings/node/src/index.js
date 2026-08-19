"use strict";

const fs = require("node:fs");
const path = require("node:path");
const koffi = require("koffi");

const EXPECTED_ABI = 1;
const finalizer = new FinalizationRegistry(({ shutdown, handle }) => { try { shutdown(handle); } catch {} });

class LicenseCoreError extends Error {
  constructor(code, message) {
    super(`License core error ${code}: ${message}`);
    this.name = "LicenseCoreError";
    this.code = code;
  }
}

function libraryName() {
  if (process.platform === "win32") return "license_core.dll";
  if (process.platform === "darwin") return "liblicense_core.dylib";
  return "liblicense_core.so";
}

function resolveLibrary(explicit) {
  const name = libraryName();
  const candidates = [
    explicit,
    path.resolve(__dirname, "../native/win-x64", name),
    path.resolve(__dirname, "../../../core/target/release", name),
  ].filter(Boolean).map(value => path.resolve(value));
  const found = candidates.find(candidate => fs.existsSync(candidate));
  if (!found) throw new Error(`${name} was not found in deterministic package/repository locations`);
  return found;
}

class NativeCore {
  constructor(nativePath) {
    this.library = koffi.load(resolveLibrary(nativePath));
    this.abiVersion = this.library.func("uint32_t license_abi_version(void)");
    this.initialize = this.library.func("int32_t license_initialize(const char *config_json, uint64_t *out_handle)");
    this.shutdown = this.library.func("int32_t license_shutdown(uint64_t handle)");
    this.activate = this.library.func("int32_t license_activate(uint64_t handle, const char *value)");
    this.getStatus = this.library.func("intptr_t license_status(uint64_t handle, void *buffer, size_t buffer_len)");
    this.requireEntitlement = this.library.func("int32_t license_require_entitlement(uint64_t handle, const char *entitlement)");
    this.refresh = this.library.func("int32_t license_refresh(uint64_t handle)");
    this.deactivate = this.library.func("int32_t license_deactivate(uint64_t handle)");
    this.getDeviceId = this.library.func("intptr_t license_device_id(uint64_t handle, void *buffer, size_t buffer_len)");
    this.getLastError = this.library.func("intptr_t license_last_error(void *buffer, size_t buffer_len)");
    const abi = this.abiVersion();
    if (abi !== EXPECTED_ABI) throw new Error(`License core ABI ${abi} is not supported; expected ${EXPECTED_ABI}`);
  }

  lastError() {
    const size = Number(this.getLastError(null, 0));
    if (size <= 1) return "unknown native error";
    const output = Buffer.alloc(size);
    if (Number(this.getLastError(output, output.length)) < 0) return "unknown native error";
    return output.subarray(0, output.indexOf(0)).toString("utf8");
  }

  check(result) {
    const code = Number(result);
    if (code < 0) throw new LicenseCoreError(-code, this.lastError());
  }

  read(fn, handle) {
    const size = Number(fn(handle, null, 0));
    this.check(size);
    const output = Buffer.alloc(size);
    const written = Number(fn(handle, output, output.length));
    this.check(written);
    const nul = output.indexOf(0);
    return output.subarray(0, nul >= 0 ? nul : output.length).toString("utf8");
  }
}

class LicenseClient {
  constructor(native, handle) {
    this._native = native;
    this._handle = handle;
    finalizer.register(this, { shutdown: native.shutdown, handle }, this);
  }

  static initialize(config, options = {}) {
    if (!config || typeof config !== "object") throw new TypeError("config is required");
    const native = new NativeCore(options.nativePath);
    const pointer = koffi.alloc("uint64_t", 1);
    native.check(native.initialize(JSON.stringify(config), pointer));
    return new LicenseClient(native, koffi.decode(pointer, "uint64_t"));
  }

  _ensureOpen() { if (this._handle === null) throw new Error("LicenseClient is closed"); }
  get deviceId() { this._ensureOpen(); return this._native.read(this._native.getDeviceId, this._handle); }
  status() { this._ensureOpen(); return JSON.parse(this._native.read(this._native.getStatus, this._handle)); }
  activate(value) {
    this._ensureOpen();
    if (typeof value !== "string" || !value.trim()) throw new TypeError("activation value is required");
    this._native.check(this._native.activate(this._handle, value));
    return this.status();
  }
  refresh() { this._ensureOpen(); this._native.check(this._native.refresh(this._handle)); return this.status(); }
  requireEntitlement(value) {
    this._ensureOpen();
    if (typeof value !== "string" || !value.trim()) throw new TypeError("entitlement is required");
    this._native.check(this._native.requireEntitlement(this._handle, value));
  }
  deactivate() { this._ensureOpen(); this._native.check(this._native.deactivate(this._handle)); }
  close() {
    if (this._handle === null) return;
    const handle = this._handle;
    this._handle = null;
    finalizer.unregister(this);
    this._native.check(this._native.shutdown(handle));
  }
}

module.exports = { LicenseClient, LicenseCoreError };
