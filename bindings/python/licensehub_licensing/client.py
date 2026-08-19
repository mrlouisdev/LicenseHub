from __future__ import annotations

import ctypes
import json
import os
import platform
import threading
import weakref
from dataclasses import dataclass
from enum import Enum
from pathlib import Path
from typing import Any, Mapping


class LicenseCoreError(RuntimeError):
    def __init__(self, code: int, message: str):
        super().__init__(f"License core error {code}: {message}")
        self.code = code


class LicenseState(str, Enum):
    ACTIVE = "active"
    EXPIRED = "expired"
    NOT_ACTIVATED = "not_activated"
    CLOCK_ROLLBACK = "clock_rollback"


@dataclass(frozen=True)
class LicenseStatus:
    state: LicenseState
    product_id: str
    device_id: str
    license_id: str | None
    entitlements: tuple[str, ...]
    issued_at: int | None
    expires_at: int | None

    @classmethod
    def from_json(cls, payload: str) -> "LicenseStatus":
        value = json.loads(payload)
        return cls(
            state=LicenseState(value["state"]), product_id=value["product_id"],
            device_id=value["device_id"], license_id=value.get("license_id"),
            entitlements=tuple(value.get("entitlements", ())),
            issued_at=value.get("issued_at"), expires_at=value.get("expires_at"),
        )


def _library_name() -> str:
    if os.name == "nt": return "license_core.dll"
    if platform.system() == "Darwin": return "liblicense_core.dylib"
    return "liblicense_core.so"


def _resolve_library(explicit: str | os.PathLike[str] | None) -> Path:
    name = _library_name()
    candidates: list[Path] = []
    if explicit is not None: candidates.append(Path(explicit))
    package = Path(__file__).resolve().parent
    candidates.extend([
        package / "_native" / "win-x64" / name,
        package.parent.parent.parent / "core" / "target" / "release" / name,
    ])
    for candidate in candidates:
        if candidate.is_file(): return candidate.resolve()
    raise FileNotFoundError(f"{name} was not found in deterministic package/repository locations")


class _Native:
    ABI = 1

    def __init__(self, path: Path):
        self.lib = ctypes.CDLL(str(path))
        self.lib.license_abi_version.argtypes = []
        self.lib.license_abi_version.restype = ctypes.c_uint32
        self.lib.license_initialize.argtypes = [ctypes.c_char_p, ctypes.POINTER(ctypes.c_uint64)]
        self.lib.license_initialize.restype = ctypes.c_int32
        self.lib.license_shutdown.argtypes = [ctypes.c_uint64]
        self.lib.license_shutdown.restype = ctypes.c_int32
        self.lib.license_activate.argtypes = [ctypes.c_uint64, ctypes.c_char_p]
        self.lib.license_activate.restype = ctypes.c_int32
        self.lib.license_status.argtypes = [ctypes.c_uint64, ctypes.c_void_p, ctypes.c_size_t]
        self.lib.license_status.restype = ctypes.c_ssize_t
        self.lib.license_require_entitlement.argtypes = [ctypes.c_uint64, ctypes.c_char_p]
        self.lib.license_require_entitlement.restype = ctypes.c_int32
        self.lib.license_refresh.argtypes = [ctypes.c_uint64]
        self.lib.license_refresh.restype = ctypes.c_int32
        self.lib.license_deactivate.argtypes = [ctypes.c_uint64]
        self.lib.license_deactivate.restype = ctypes.c_int32
        self.lib.license_device_id.argtypes = [ctypes.c_uint64, ctypes.c_void_p, ctypes.c_size_t]
        self.lib.license_device_id.restype = ctypes.c_ssize_t
        self.lib.license_last_error.argtypes = [ctypes.c_void_p, ctypes.c_size_t]
        self.lib.license_last_error.restype = ctypes.c_ssize_t
        abi = int(self.lib.license_abi_version())
        if abi != self.ABI: raise RuntimeError(f"License core ABI {abi} is not supported; expected {self.ABI}")

    def last_error(self) -> str:
        size = int(self.lib.license_last_error(None, 0))
        if size <= 1: return "unknown native error"
        buffer = ctypes.create_string_buffer(size)
        if self.lib.license_last_error(buffer, size) < 0: return "unknown native error"
        return buffer.value.decode("utf-8")

    def check(self, result: int) -> None:
        if result < 0: raise LicenseCoreError(-result, self.last_error())

    def read(self, function: Any, handle: int) -> str:
        size = int(function(handle, None, 0))
        self.check(size)
        buffer = ctypes.create_string_buffer(size)
        written = int(function(handle, buffer, size))
        self.check(written)
        return buffer.value.decode("utf-8")


class LicenseClient:
    def __init__(self, native: _Native, handle: int):
        self._native, self._handle = native, handle
        self._lock = threading.RLock()
        self._finalizer = weakref.finalize(self, native.lib.license_shutdown, handle)

    @classmethod
    def initialize(cls, config: Mapping[str, Any], *, native_path: str | os.PathLike[str] | None = None) -> "LicenseClient":
        native = _Native(_resolve_library(native_path))
        payload = json.dumps(dict(config), separators=(",", ":")).encode("utf-8")
        handle = ctypes.c_uint64()
        native.check(int(native.lib.license_initialize(payload, ctypes.byref(handle))))
        return cls(native, int(handle.value))

    def _ensure_open(self) -> None:
        if self._handle == 0: raise RuntimeError("LicenseClient is closed")

    @property
    def device_id(self) -> str:
        with self._lock:
            self._ensure_open()
            return self._native.read(self._native.lib.license_device_id, self._handle)

    def status(self) -> LicenseStatus:
        with self._lock:
            self._ensure_open()
            return LicenseStatus.from_json(self._native.read(self._native.lib.license_status, self._handle))

    def activate(self, value: str) -> LicenseStatus:
        if not value.strip(): raise ValueError("activation value is required")
        with self._lock:
            self._ensure_open()
            self._native.check(int(self._native.lib.license_activate(self._handle, value.encode("utf-8"))))
            return self.status()

    def refresh(self) -> LicenseStatus:
        with self._lock:
            self._ensure_open()
            self._native.check(int(self._native.lib.license_refresh(self._handle)))
            return self.status()

    def require_entitlement(self, entitlement: str) -> None:
        if not entitlement.strip(): raise ValueError("entitlement is required")
        with self._lock:
            self._ensure_open()
            self._native.check(int(self._native.lib.license_require_entitlement(self._handle, entitlement.encode("utf-8"))))

    def deactivate(self) -> None:
        with self._lock:
            self._ensure_open()
            self._native.check(int(self._native.lib.license_deactivate(self._handle)))

    def close(self) -> None:
        with self._lock:
            if self._handle:
                handle, self._handle = self._handle, 0
                self._finalizer.detach()
                self._native.check(int(self._native.lib.license_shutdown(handle)))

    def __enter__(self) -> "LicenseClient": return self
    def __exit__(self, *_: object) -> None: self.close()
