use std::{
    cell::RefCell,
    collections::HashMap,
    ffi::{c_char, CStr},
    path::PathBuf,
    ptr,
    sync::{
        atomic::{AtomicU64, Ordering},
        Arc, Mutex, OnceLock,
    },
    time::Duration,
};

use serde::Deserialize;

use crate::{
    client::{ClientConfig, LicenseClient},
    clock::SystemClock,
    error::{LicenseError, Result},
    store::{FileSecureStore, SecureStore},
    transport::HttpTransport,
};

pub const ABI_VERSION: u32 = 1;
type Handle = u64;

static NEXT_HANDLE: AtomicU64 = AtomicU64::new(1);
static CLIENTS: OnceLock<Mutex<HashMap<Handle, LicenseClient>>> = OnceLock::new();
thread_local! { static LAST_ERROR: RefCell<String> = const { RefCell::new(String::new()) }; }

fn clients() -> &'static Mutex<HashMap<Handle, LicenseClient>> {
    CLIENTS.get_or_init(|| Mutex::new(HashMap::new()))
}

#[derive(Deserialize)]
struct FfiConfig {
    #[serde(flatten)]
    client: ClientConfig,
    server_url: String,
    cache_dir: PathBuf,
    #[serde(default = "default_timeout")]
    request_timeout_seconds: u64,
    #[serde(default)]
    allow_insecure_localhost: bool,
}
fn default_timeout() -> u64 {
    15
}

fn fail(error: LicenseError) -> i32 {
    let code = error.code();
    LAST_ERROR.with(|slot| *slot.borrow_mut() = error.to_string());
    -code
}
fn ok() -> i32 {
    LAST_ERROR.with(|slot| slot.borrow_mut().clear());
    0
}

unsafe fn required_str<'a>(value: *const c_char, name: &str) -> Result<&'a str> {
    if value.is_null() {
        return Err(LicenseError::InvalidArgument(format!("{name} is null")));
    }
    unsafe { CStr::from_ptr(value) }
        .to_str()
        .map_err(|_| LicenseError::InvalidArgument(format!("{name} is not UTF-8")))
}

unsafe fn write_output(value: &str, buffer: *mut c_char, buffer_len: usize) -> Result<usize> {
    let required = value
        .len()
        .checked_add(1)
        .ok_or_else(|| LicenseError::InvalidArgument("output length overflow".into()))?;
    if buffer.is_null() || buffer_len == 0 {
        return Ok(required);
    }
    if buffer_len < required {
        return Err(LicenseError::InvalidArgument(format!(
            "output buffer needs {required} bytes"
        )));
    }
    unsafe {
        ptr::copy_nonoverlapping(value.as_ptr(), buffer.cast::<u8>(), value.len());
        *buffer.add(value.len()) = 0;
    }
    Ok(required)
}

fn with_client<T>(handle: Handle, f: impl FnOnce(&mut LicenseClient) -> Result<T>) -> Result<T> {
    let mut map = clients()
        .lock()
        .map_err(|_| LicenseError::Internal("client registry lock poisoned".into()))?;
    let client = map
        .get_mut(&handle)
        .ok_or_else(|| LicenseError::InvalidArgument("invalid license handle".into()))?;
    f(client)
}

#[no_mangle]
pub extern "C" fn license_abi_version() -> u32 {
    ABI_VERSION
}

/// Creates a client and writes its opaque handle to `out_handle`.
///
/// # Safety
///
/// `config_json` must point to a readable NUL-terminated UTF-8 string for the
/// duration of this call. `out_handle` must point to writable, aligned storage
/// for one `u64`.
#[no_mangle]
pub unsafe extern "C" fn license_initialize(
    config_json: *const c_char,
    out_handle: *mut Handle,
) -> i32 {
    let result = (|| {
        if out_handle.is_null() {
            return Err(LicenseError::InvalidArgument("out_handle is null".into()));
        }
        let json = unsafe { required_str(config_json, "config_json") }?;
        let config: FfiConfig = serde_json::from_str(json)
            .map_err(|e| LicenseError::Configuration(format!("config JSON: {e}")))?;
        if !(1..=120).contains(&config.request_timeout_seconds) {
            return Err(LicenseError::Configuration(
                "request_timeout_seconds must be between 1 and 120".into(),
            ));
        }
        let transport = Arc::new(HttpTransport::new(
            &config.server_url,
            Duration::from_secs(config.request_timeout_seconds),
            config.allow_insecure_localhost,
        )?);
        #[cfg(windows)]
        let store: Arc<dyn SecureStore> = Arc::new(FileSecureStore::new(
            config.cache_dir,
            crate::store::DpapiProtector,
        )?);
        #[cfg(not(windows))]
        let store: Arc<dyn SecureStore> = {
            let _ = config.cache_dir;
            return Err(LicenseError::Configuration(
                "default secure store requires Windows DPAPI; inject SecureStore through the Rust API".into(),
            ));
        };
        let client =
            LicenseClient::initialize(config.client, transport, store, Arc::new(SystemClock))?;
        let handle = NEXT_HANDLE.fetch_add(1, Ordering::Relaxed);
        clients()
            .lock()
            .map_err(|_| LicenseError::Internal("client registry lock poisoned".into()))?
            .insert(handle, client);
        unsafe {
            *out_handle = handle;
        }
        Ok(())
    })();
    match result {
        Ok(()) => ok(),
        Err(e) => fail(e),
    }
}

#[no_mangle]
pub extern "C" fn license_shutdown(handle: Handle) -> i32 {
    match clients().lock() {
        Ok(mut map) => match map.remove(&handle) {
            Some(_) => ok(),
            None => fail(LicenseError::InvalidArgument(
                "invalid license handle".into(),
            )),
        },
        Err(_) => fail(LicenseError::Internal(
            "client registry lock poisoned".into(),
        )),
    }
}

/// Activates a license for an initialized client.
///
/// # Safety
///
/// `license_key` must point to a readable NUL-terminated UTF-8 string for the
/// duration of this call.
#[no_mangle]
pub unsafe extern "C" fn license_activate(handle: Handle, license_key: *const c_char) -> i32 {
    let result = unsafe { required_str(license_key, "license_key") }
        .and_then(|key| with_client(handle, |client| client.activate(key).map(|_| ())));
    match result {
        Ok(()) => ok(),
        Err(e) => fail(e),
    }
}

/// Serializes the current status as UTF-8 JSON.
///
/// # Safety
///
/// `buffer` may be null to query the required size. Otherwise it must point to
/// at least `buffer_len` writable bytes.
#[no_mangle]
pub unsafe extern "C" fn license_status(
    handle: Handle,
    buffer: *mut c_char,
    buffer_len: usize,
) -> isize {
    let result = with_client(handle, |client| {
        let status = client.status()?;
        serde_json::to_string(&status).map_err(|e| LicenseError::Internal(e.to_string()))
    })
    .and_then(|json| unsafe { write_output(&json, buffer, buffer_len) });
    match result {
        Ok(n) => n as isize,
        Err(e) => fail(e) as isize,
    }
}

/// Alias of [`license_status`].
///
/// # Safety
///
/// `buffer` may be null to query the required size. Otherwise it must point to
/// at least `buffer_len` writable bytes.
#[no_mangle]
pub unsafe extern "C" fn license_check(
    handle: Handle,
    buffer: *mut c_char,
    buffer_len: usize,
) -> isize {
    unsafe { license_status(handle, buffer, buffer_len) }
}

/// Requires an entitlement on the current active lease.
///
/// # Safety
///
/// `entitlement` must point to a readable NUL-terminated UTF-8 string for the
/// duration of this call.
#[no_mangle]
pub unsafe extern "C" fn license_require_entitlement(
    handle: Handle,
    entitlement: *const c_char,
) -> i32 {
    let result = unsafe { required_str(entitlement, "entitlement") }
        .and_then(|name| with_client(handle, |client| client.require_entitlement(name)));
    match result {
        Ok(()) => ok(),
        Err(e) => fail(e),
    }
}

#[no_mangle]
pub extern "C" fn license_refresh(handle: Handle) -> i32 {
    match with_client(handle, |client| client.refresh().map(|_| ())) {
        Ok(()) => ok(),
        Err(e) => fail(e),
    }
}

#[no_mangle]
pub extern "C" fn license_deactivate(handle: Handle) -> i32 {
    match with_client(handle, LicenseClient::deactivate) {
        Ok(()) => ok(),
        Err(e) => fail(e),
    }
}

/// Copies the stable device identifier into a caller-owned buffer.
///
/// # Safety
///
/// `buffer` may be null to query the required size. Otherwise it must point to
/// at least `buffer_len` writable bytes.
#[no_mangle]
pub unsafe extern "C" fn license_device_id(
    handle: Handle,
    buffer: *mut c_char,
    buffer_len: usize,
) -> isize {
    let result = with_client(handle, |client| Ok(client.device_id().to_owned()))
        .and_then(|value| unsafe { write_output(&value, buffer, buffer_len) });
    match result {
        Ok(n) => n as isize,
        Err(e) => fail(e) as isize,
    }
}

/// Copies the current thread's last FFI error into a caller-owned buffer.
///
/// # Safety
///
/// `buffer` may be null to query the required size. Otherwise it must point to
/// at least `buffer_len` writable bytes.
#[no_mangle]
pub unsafe extern "C" fn license_last_error(buffer: *mut c_char, buffer_len: usize) -> isize {
    let message = LAST_ERROR.with(|slot| slot.borrow().clone());
    match unsafe { write_output(&message, buffer, buffer_len) } {
        Ok(n) => n as isize,
        Err(e) => fail(e) as isize,
    }
}
