use std::{collections::HashMap, sync::Arc};

use base64::{engine::general_purpose::STANDARD, Engine as _};
use ed25519_dalek::VerifyingKey;
use rand::RngCore;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

use crate::{
    clock::Clock,
    error::{LicenseError, Result},
    lease::{verify_token, LeaseClaims, VerifiedLease},
    store::SecureStore,
    transport::{ActivateRequest, DeactivateRequest, LicenseTransport, RefreshRequest},
};

const DEVICE_SEED_KEY: &str = "device_seed";
const LEASE_KEY: &str = "lease";
const LAST_SEEN_KEY: &str = "last_seen";

#[derive(Debug, Clone, Deserialize)]
pub struct ClientConfig {
    pub product_id: String,
    /// Immutable trust pins: key id to standard-base64 encoded 32-byte Ed25519
    /// public key. Rotation is staged by shipping both current and next keys in
    /// an authenticated application/SDK update before the server switches.
    ///
    /// Never populate or replace this map directly from the public-keys HTTP
    /// endpoint: that would let a compromised endpoint authorize its own key.
    pub public_keys: HashMap<String, String>,
    #[serde(default = "default_clock_tolerance")]
    pub clock_rollback_tolerance_seconds: i64,
}

fn default_clock_tolerance() -> i64 {
    300
}

impl ClientConfig {
    pub fn decode_keys(&self) -> Result<HashMap<String, VerifyingKey>> {
        if self.product_id.trim().is_empty() {
            return Err(LicenseError::Configuration("product_id is required".into()));
        }
        if self.public_keys.is_empty() {
            return Err(LicenseError::Configuration(
                "at least one public key is required".into(),
            ));
        }
        if self.clock_rollback_tolerance_seconds < 0 {
            return Err(LicenseError::Configuration(
                "clock tolerance cannot be negative".into(),
            ));
        }
        self.public_keys
            .iter()
            .map(|(kid, encoded)| {
                let bytes = STANDARD.decode(encoded).map_err(|e| {
                    LicenseError::Configuration(format!("public key '{kid}' is not base64: {e}"))
                })?;
                let bytes: [u8; 32] = bytes.try_into().map_err(|_| {
                    LicenseError::Configuration(format!("public key '{kid}' must be 32 bytes"))
                })?;
                let key = VerifyingKey::from_bytes(&bytes).map_err(|e| {
                    LicenseError::Configuration(format!("public key '{kid}' is invalid: {e}"))
                })?;
                Ok((kid.clone(), key))
            })
            .collect()
    }

    /// Sorted identifiers of the keys pinned by the authenticated client
    /// configuration. Useful for rotation-readiness telemetry without exposing
    /// key bytes.
    pub fn pinned_key_ids(&self) -> Vec<String> {
        let mut ids: Vec<_> = self.public_keys.keys().cloned().collect();
        ids.sort();
        ids
    }
}

#[derive(Debug, Clone, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum LicenseState {
    Active,
    Expired,
    NotActivated,
    ClockRollback,
}

#[derive(Debug, Clone, Serialize)]
pub struct LicenseStatus {
    pub state: LicenseState,
    pub product_id: String,
    pub device_id: String,
    pub license_id: Option<String>,
    pub entitlements: Vec<String>,
    pub issued_at: Option<i64>,
    pub expires_at: Option<i64>,
}

pub struct LicenseClient {
    config: ClientConfig,
    keys: HashMap<String, VerifyingKey>,
    transport: Arc<dyn LicenseTransport>,
    store: Arc<dyn SecureStore>,
    clock: Arc<dyn Clock>,
    device_id: String,
    lease: Option<VerifiedLease>,
    last_error: Option<String>,
}

impl LicenseClient {
    pub fn initialize(
        config: ClientConfig,
        transport: Arc<dyn LicenseTransport>,
        store: Arc<dyn SecureStore>,
        clock: Arc<dyn Clock>,
    ) -> Result<Self> {
        let keys = config.decode_keys()?;
        let seed = match store.get(DEVICE_SEED_KEY)? {
            Some(seed) if seed.len() == 32 => seed,
            Some(_) => return Err(LicenseError::Storage("device seed is corrupt".into())),
            None => {
                let mut seed = vec![0u8; 32];
                rand::rngs::OsRng.fill_bytes(&mut seed);
                store.set(DEVICE_SEED_KEY, &seed)?;
                seed
            }
        };
        let mut hasher = Sha256::new();
        hasher.update(b"license-core-device-v1\0");
        hasher.update(config.product_id.as_bytes());
        hasher.update([0]);
        hasher.update(seed);
        let device_id = format!("dev_{:x}", hasher.finalize());

        let mut client = Self {
            config,
            keys,
            transport,
            store,
            clock,
            device_id,
            lease: None,
            last_error: None,
        };
        if let Some(token) = client.store.get(LEASE_KEY)? {
            let token = String::from_utf8(token)
                .map_err(|_| LicenseError::Storage("cached lease is not UTF-8".into()))?;
            client.lease = Some(client.verify_bound_lease(token)?);
        }
        Ok(client)
    }

    pub fn device_id(&self) -> &str {
        &self.device_id
    }
    pub fn last_error(&self) -> Option<&str> {
        self.last_error.as_deref()
    }

    fn record<T>(&mut self, result: Result<T>) -> Result<T> {
        match result {
            Ok(value) => {
                self.last_error = None;
                Ok(value)
            }
            Err(error) => {
                self.last_error = Some(error.to_string());
                Err(error)
            }
        }
    }

    fn verify_bound_lease(&self, token: String) -> Result<VerifiedLease> {
        let claims = verify_token(&token, &self.keys)?;
        if claims.product_id != self.config.product_id {
            return Err(LicenseError::ProductMismatch {
                expected: self.config.product_id.clone(),
                actual: claims.product_id,
            });
        }
        if claims.device_id != self.device_id {
            return Err(LicenseError::DeviceMismatch);
        }
        Ok(VerifiedLease { token, claims })
    }

    fn checked_now(&self) -> Result<i64> {
        let now = self.clock.unix_seconds()?;
        let last_seen = self
            .store
            .get(LAST_SEEN_KEY)?
            .map(|v| {
                String::from_utf8(v)
                    .map_err(|_| LicenseError::Storage("last_seen is corrupt".into()))
            })
            .transpose()?
            .map(|v| {
                v.parse::<i64>()
                    .map_err(|_| LicenseError::Storage("last_seen is corrupt".into()))
            })
            .transpose()?;
        if let Some(last_seen) = last_seen {
            if now.saturating_add(self.config.clock_rollback_tolerance_seconds) < last_seen {
                return Err(LicenseError::ClockRollback { now, last_seen });
            }
        }
        if last_seen.map(|value| now > value).unwrap_or(true) {
            self.store.set(LAST_SEEN_KEY, now.to_string().as_bytes())?;
        }
        Ok(now)
    }

    fn ensure_active(&self) -> Result<&LeaseClaims> {
        let now = self.checked_now()?;
        let claims = &self
            .lease
            .as_ref()
            .ok_or(LicenseError::NotActivated)?
            .claims;
        if claims.iat > now.saturating_add(self.config.clock_rollback_tolerance_seconds) {
            return Err(LicenseError::InvalidLease(
                "lease issued too far in the future".into(),
            ));
        }
        if now >= claims.exp {
            return Err(LicenseError::Expired(claims.exp));
        }
        Ok(claims)
    }

    pub fn activate(&mut self, license_key: &str) -> Result<LicenseStatus> {
        if license_key.trim().is_empty() {
            return self.record(Err(LicenseError::InvalidArgument(
                "license_key is required".into(),
            )));
        }
        let result = (|| {
            let token = self.transport.activate(ActivateRequest {
                product_id: &self.config.product_id,
                license_key,
                device_id: &self.device_id,
            })?;
            let lease = self.verify_bound_lease(token)?;
            self.store.set(LEASE_KEY, lease.token.as_bytes())?;
            self.lease = Some(lease);
            self.status_inner()
        })();
        self.record(result)
    }

    pub fn refresh(&mut self) -> Result<LicenseStatus> {
        let result = (|| {
            let current = self
                .lease
                .as_ref()
                .ok_or(LicenseError::NotActivated)?
                .token
                .clone();
            let token = self.transport.refresh(RefreshRequest {
                product_id: &self.config.product_id,
                device_id: &self.device_id,
                lease: &current,
            })?;
            let lease = self.verify_bound_lease(token)?;
            self.store.set(LEASE_KEY, lease.token.as_bytes())?;
            self.lease = Some(lease);
            self.status_inner()
        })();
        self.record(result)
    }

    pub fn deactivate(&mut self) -> Result<()> {
        let result = (|| {
            if let Some(lease) = &self.lease {
                self.transport.deactivate(DeactivateRequest {
                    product_id: &self.config.product_id,
                    device_id: &self.device_id,
                    lease: &lease.token,
                })?;
            }
            self.store.delete(LEASE_KEY)?;
            self.lease = None;
            Ok(())
        })();
        self.record(result)
    }

    fn status_inner(&self) -> Result<LicenseStatus> {
        let base = |state, claims: Option<&LeaseClaims>| LicenseStatus {
            state,
            product_id: self.config.product_id.clone(),
            device_id: self.device_id.clone(),
            license_id: claims.map(|c| c.license_id.clone()),
            entitlements: claims
                .map(|c| c.entitlements.iter().cloned().collect())
                .unwrap_or_default(),
            issued_at: claims.map(|c| c.iat),
            expires_at: claims.map(|c| c.exp),
        };
        let claims = match &self.lease {
            Some(lease) => &lease.claims,
            None => return Ok(base(LicenseState::NotActivated, None)),
        };
        match self.ensure_active() {
            Ok(_) => Ok(base(LicenseState::Active, Some(claims))),
            Err(LicenseError::Expired(_)) => Ok(base(LicenseState::Expired, Some(claims))),
            Err(LicenseError::ClockRollback { .. }) => {
                Ok(base(LicenseState::ClockRollback, Some(claims)))
            }
            Err(e) => Err(e),
        }
    }

    pub fn status(&mut self) -> Result<LicenseStatus> {
        let result = self.status_inner();
        self.record(result)
    }

    pub fn require_entitlement(&mut self, entitlement: &str) -> Result<()> {
        let result = (|| {
            if entitlement.trim().is_empty() {
                return Err(LicenseError::InvalidArgument(
                    "entitlement is required".into(),
                ));
            }
            let claims = self.ensure_active()?;
            if !claims.entitlements.contains(entitlement) {
                return Err(LicenseError::EntitlementMissing(entitlement.into()));
            }
            Ok(())
        })();
        self.record(result)
    }
}
