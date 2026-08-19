use std::collections::{BTreeSet, HashMap};

use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine as _};
use ed25519_dalek::{Signature, Verifier, VerifyingKey};
use serde::{Deserialize, Serialize};

use crate::error::{LicenseError, Result};

pub const LEASE_VERSION: u16 = 1;

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct LeaseClaims {
    pub version: u16,
    pub kid: String,
    pub license_id: String,
    pub product_id: String,
    pub device_id: String,
    #[serde(default)]
    pub entitlements: BTreeSet<String>,
    pub iat: i64,
    pub exp: i64,
}

#[derive(Debug, Clone)]
pub struct VerifiedLease {
    pub token: String,
    pub claims: LeaseClaims,
}

/// Format: `base64url(payload).base64url(ed25519 signature)`.
/// The signature covers the encoded payload segment verbatim.
pub fn verify_token(token: &str, keys: &HashMap<String, VerifyingKey>) -> Result<LeaseClaims> {
    let (payload_segment, signature_segment) = token
        .split_once('.')
        .ok_or_else(|| LicenseError::InvalidLease("expected two token segments".into()))?;
    if signature_segment.contains('.') {
        return Err(LicenseError::InvalidLease(
            "expected two token segments".into(),
        ));
    }
    let payload = URL_SAFE_NO_PAD
        .decode(payload_segment)
        .map_err(|e| LicenseError::InvalidLease(format!("payload is not base64url: {e}")))?;
    let claims: LeaseClaims = serde_json::from_slice(&payload)
        .map_err(|e| LicenseError::InvalidLease(format!("payload is not valid JSON: {e}")))?;
    if claims.version != LEASE_VERSION {
        return Err(LicenseError::InvalidLease(format!(
            "unsupported lease version {}",
            claims.version
        )));
    }
    if claims.exp <= claims.iat {
        return Err(LicenseError::InvalidLease(
            "exp must be later than iat".into(),
        ));
    }
    let key = keys.get(&claims.kid).ok_or_else(|| {
        LicenseError::InvalidLease(format!("unknown signing key '{}'", claims.kid))
    })?;
    let signature_bytes = URL_SAFE_NO_PAD
        .decode(signature_segment)
        .map_err(|e| LicenseError::InvalidLease(format!("signature is not base64url: {e}")))?;
    let signature = Signature::from_slice(&signature_bytes)
        .map_err(|_| LicenseError::InvalidLease("signature must be 64 bytes".into()))?;
    key.verify(payload_segment.as_bytes(), &signature)
        .map_err(|_| LicenseError::InvalidSignature)?;
    Ok(claims)
}
