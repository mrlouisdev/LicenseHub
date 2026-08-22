use std::collections::{BTreeSet, HashMap};

use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine as _};
use ed25519_dalek::{Signature, Verifier, VerifyingKey};
use serde::{Deserialize, Serialize};

use crate::error::{LicenseError, Result};

pub const LEASE_VERSION: u16 = 1;
pub const MAX_SIGNED_LEASE_BYTES: usize = 48 * 1024;
const MAX_LEASE_PAYLOAD_BYTES: usize = 32 * 1024;
const MAX_ENTITLEMENTS: usize = 512;

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
    if token.is_empty() || token.len() > MAX_SIGNED_LEASE_BYTES {
        return Err(LicenseError::InvalidLease("lease length is invalid".into()));
    }
    let (payload_segment, signature_segment) = token
        .split_once('.')
        .ok_or_else(|| LicenseError::InvalidLease("expected two token segments".into()))?;
    if signature_segment.contains('.') {
        return Err(LicenseError::InvalidLease(
            "expected two token segments".into(),
        ));
    }
    if payload_segment.is_empty() || signature_segment.is_empty() {
        return Err(LicenseError::InvalidLease(
            "expected two non-empty token segments".into(),
        ));
    }
    let payload = URL_SAFE_NO_PAD
        .decode(payload_segment)
        .map_err(|e| LicenseError::InvalidLease(format!("payload is not base64url: {e}")))?;
    if payload.len() > MAX_LEASE_PAYLOAD_BYTES {
        return Err(LicenseError::InvalidLease(
            "lease payload is too large".into(),
        ));
    }
    #[derive(Deserialize)]
    struct UntrustedHeader {
        kid: String,
    }
    let header: UntrustedHeader = serde_json::from_slice(&payload)
        .map_err(|e| LicenseError::InvalidLease(format!("payload is not valid JSON: {e}")))?;
    if !valid_key_id(&header.kid) {
        return Err(LicenseError::InvalidLease("token key id is invalid".into()));
    }
    let key = keys.get(&header.kid).ok_or_else(|| {
        LicenseError::InvalidLease(format!("unknown signing key '{}'", header.kid))
    })?;
    let signature_bytes = URL_SAFE_NO_PAD
        .decode(signature_segment)
        .map_err(|e| LicenseError::InvalidLease(format!("signature is not base64url: {e}")))?;
    let signature = Signature::from_slice(&signature_bytes)
        .map_err(|_| LicenseError::InvalidLease("signature must be 64 bytes".into()))?;
    key.verify(payload_segment.as_bytes(), &signature)
        .map_err(|_| LicenseError::InvalidSignature)?;
    let claims: LeaseClaims = serde_json::from_slice(&payload)
        .map_err(|e| LicenseError::InvalidLease(format!("payload is not valid JSON: {e}")))?;
    if claims.version != LEASE_VERSION {
        return Err(LicenseError::InvalidLease(format!(
            "unsupported lease version {}",
            claims.version
        )));
    }
    if claims.kid != header.kid || claims.exp <= claims.iat || claims.iat < 0 {
        return Err(LicenseError::InvalidLease(
            "lease claims are inconsistent".into(),
        ));
    }
    if claims.license_id.is_empty()
        || claims.license_id.len() > 128
        || claims.product_id.is_empty()
        || claims.product_id.len() > 128
        || claims.device_id.is_empty()
        || claims.device_id.len() > 256
        || claims.entitlements.len() > MAX_ENTITLEMENTS
        || claims
            .entitlements
            .iter()
            .any(|value| value.is_empty() || value.len() > 256)
    {
        return Err(LicenseError::InvalidLease(
            "lease claim bounds are invalid".into(),
        ));
    }
    Ok(claims)
}

pub(crate) fn valid_key_id(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 64
        && value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'.' | b'_' | b'-'))
}
