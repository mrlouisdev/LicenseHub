use std::{
    fs::{self, OpenOptions},
    io::Write,
    path::{Path, PathBuf},
};

use rand::{distributions::Alphanumeric, Rng};

use crate::error::{LicenseError, Result};

#[cfg(windows)]
pub use windows_dpapi::DpapiProtector;

pub trait SecureStore: Send + Sync {
    fn get(&self, key: &str) -> Result<Option<Vec<u8>>>;
    fn set(&self, key: &str, value: &[u8]) -> Result<()>;
    fn delete(&self, key: &str) -> Result<()>;
}

pub trait SecretProtector: Send + Sync {
    fn protect(&self, plaintext: &[u8]) -> Result<Vec<u8>>;
    fn unprotect(&self, ciphertext: &[u8]) -> Result<Vec<u8>>;
}

#[cfg(windows)]
mod windows_dpapi {
    use super::SecretProtector;
    use crate::error::{LicenseError, Result};
    use std::ptr;
    use windows_sys::Win32::{
        Foundation::LocalFree,
        Security::Cryptography::{
            CryptProtectData, CryptUnprotectData, CRYPTPROTECT_UI_FORBIDDEN, CRYPT_INTEGER_BLOB,
        },
    };

    #[derive(Debug, Default, Clone, Copy)]
    pub struct DpapiProtector;

    impl DpapiProtector {
        fn transform(input: &[u8], protect: bool) -> Result<Vec<u8>> {
            let input_len = u32::try_from(input.len())
                .map_err(|_| LicenseError::Storage("value is too large for DPAPI".into()))?;
            let input_blob = CRYPT_INTEGER_BLOB {
                cbData: input_len,
                pbData: input.as_ptr() as *mut u8,
            };
            let mut output_blob = CRYPT_INTEGER_BLOB {
                cbData: 0,
                pbData: ptr::null_mut(),
            };
            let ok = unsafe {
                if protect {
                    CryptProtectData(
                        &input_blob,
                        ptr::null(),
                        ptr::null(),
                        ptr::null_mut(),
                        ptr::null_mut(),
                        CRYPTPROTECT_UI_FORBIDDEN,
                        &mut output_blob,
                    )
                } else {
                    CryptUnprotectData(
                        &input_blob,
                        ptr::null_mut(),
                        ptr::null(),
                        ptr::null_mut(),
                        ptr::null_mut(),
                        CRYPTPROTECT_UI_FORBIDDEN,
                        &mut output_blob,
                    )
                }
            };
            if ok == 0 {
                return Err(LicenseError::Storage(format!(
                    "Windows DPAPI operation failed: {}",
                    std::io::Error::last_os_error()
                )));
            }
            let output = unsafe {
                let slice =
                    std::slice::from_raw_parts(output_blob.pbData, output_blob.cbData as usize);
                let owned = slice.to_vec();
                LocalFree(output_blob.pbData.cast());
                owned
            };
            Ok(output)
        }
    }

    impl SecretProtector for DpapiProtector {
        fn protect(&self, plaintext: &[u8]) -> Result<Vec<u8>> {
            Self::transform(plaintext, true)
        }
        fn unprotect(&self, ciphertext: &[u8]) -> Result<Vec<u8>> {
            Self::transform(ciphertext, false)
        }
    }
}

pub struct FileSecureStore<P: SecretProtector> {
    root: PathBuf,
    protector: P,
}

impl<P: SecretProtector> FileSecureStore<P> {
    pub fn new(root: impl Into<PathBuf>, protector: P) -> Result<Self> {
        let root = root.into();
        fs::create_dir_all(&root)
            .map_err(|e| LicenseError::Storage(format!("create {}: {e}", root.display())))?;
        Ok(Self { root, protector })
    }

    fn path(&self, key: &str) -> Result<PathBuf> {
        let valid = !key.is_empty()
            && key
                .bytes()
                .all(|b| b.is_ascii_alphanumeric() || matches!(b, b'-' | b'_'));
        if !valid {
            return Err(LicenseError::Storage("invalid storage key".into()));
        }
        Ok(self.root.join(key))
    }

    fn atomic_write(path: &Path, bytes: &[u8]) -> Result<()> {
        let suffix: String = rand::thread_rng()
            .sample_iter(&Alphanumeric)
            .take(12)
            .map(char::from)
            .collect();
        let temp = path.with_extension(format!("tmp-{suffix}"));
        let result = (|| {
            let mut file = OpenOptions::new()
                .create_new(true)
                .write(true)
                .open(&temp)
                .map_err(|e| LicenseError::Storage(format!("create {}: {e}", temp.display())))?;
            file.write_all(bytes)
                .and_then(|_| file.sync_all())
                .map_err(|e| LicenseError::Storage(format!("write {}: {e}", temp.display())))?;
            replace_file(&temp, path)
        })();
        if result.is_err() {
            let _ = fs::remove_file(&temp);
        }
        result
    }
}

impl<P: SecretProtector> SecureStore for FileSecureStore<P> {
    fn get(&self, key: &str) -> Result<Option<Vec<u8>>> {
        let path = self.path(key)?;
        match fs::read(&path) {
            Ok(bytes) => self.protector.unprotect(&bytes).map(Some),
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(None),
            Err(e) => Err(LicenseError::Storage(format!(
                "read {}: {e}",
                path.display()
            ))),
        }
    }

    fn set(&self, key: &str, value: &[u8]) -> Result<()> {
        let path = self.path(key)?;
        Self::atomic_write(&path, &self.protector.protect(value)?)
    }

    fn delete(&self, key: &str) -> Result<()> {
        let path = self.path(key)?;
        match fs::remove_file(&path) {
            Ok(()) => Ok(()),
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()),
            Err(e) => Err(LicenseError::Storage(format!(
                "delete {}: {e}",
                path.display()
            ))),
        }
    }
}

#[cfg(not(windows))]
fn replace_file(source: &Path, destination: &Path) -> Result<()> {
    fs::rename(source, destination)
        .map_err(|e| LicenseError::Storage(format!("replace {}: {e}", destination.display())))
}

#[cfg(windows)]
fn replace_file(source: &Path, destination: &Path) -> Result<()> {
    use std::os::windows::ffi::OsStrExt;
    use windows_sys::Win32::Storage::FileSystem::{
        MoveFileExW, MOVEFILE_REPLACE_EXISTING, MOVEFILE_WRITE_THROUGH,
    };
    let source: Vec<u16> = source.as_os_str().encode_wide().chain(Some(0)).collect();
    let destination: Vec<u16> = destination
        .as_os_str()
        .encode_wide()
        .chain(Some(0))
        .collect();
    let ok = unsafe {
        MoveFileExW(
            source.as_ptr(),
            destination.as_ptr(),
            MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH,
        )
    };
    if ok == 0 {
        return Err(LicenseError::Storage(format!(
            "atomic replace failed: {}",
            std::io::Error::last_os_error()
        )));
    }
    Ok(())
}

#[cfg(test)]
pub mod testing {
    use super::SecureStore;
    use crate::error::{LicenseError, Result};
    use std::{collections::HashMap, sync::Mutex};

    #[derive(Default)]
    pub struct MemoryStore(pub Mutex<HashMap<String, Vec<u8>>>);
    impl SecureStore for MemoryStore {
        fn get(&self, key: &str) -> Result<Option<Vec<u8>>> {
            Ok(self
                .0
                .lock()
                .map_err(|_| LicenseError::Storage("lock poisoned".into()))?
                .get(key)
                .cloned())
        }
        fn set(&self, key: &str, value: &[u8]) -> Result<()> {
            self.0
                .lock()
                .map_err(|_| LicenseError::Storage("lock poisoned".into()))?
                .insert(key.into(), value.to_vec());
            Ok(())
        }
        fn delete(&self, key: &str) -> Result<()> {
            self.0
                .lock()
                .map_err(|_| LicenseError::Storage("lock poisoned".into()))?
                .remove(key);
            Ok(())
        }
    }
}

#[cfg(all(test, windows))]
mod windows_tests {
    use super::{DpapiProtector, FileSecureStore, SecureStore};
    use rand::{distributions::Alphanumeric, Rng};
    use std::fs;

    #[test]
    fn dpapi_store_round_trips_and_replaces_atomically() {
        let suffix: String = rand::thread_rng()
            .sample_iter(&Alphanumeric)
            .take(12)
            .map(char::from)
            .collect();
        let root = std::env::temp_dir().join(format!("license-core-{suffix}"));
        let store = FileSecureStore::new(&root, DpapiProtector).unwrap();
        store.set("lease", b"first").unwrap();
        store.set("lease", b"second").unwrap();
        assert_eq!(store.get("lease").unwrap(), Some(b"second".to_vec()));
        store.delete("lease").unwrap();
        assert_eq!(store.get("lease").unwrap(), None);
        fs::remove_dir_all(root).unwrap();
    }
}
