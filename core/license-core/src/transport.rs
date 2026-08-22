use std::{io::Read, time::Duration};

use reqwest::blocking::Client;
use serde::{Deserialize, Serialize};

use crate::error::{LicenseError, Result};

#[derive(Debug, Clone, Serialize)]
pub struct ActivateRequest<'a> {
    pub product_id: &'a str,
    pub license_key: &'a str,
    pub device_id: &'a str,
}

#[derive(Debug, Clone, Serialize)]
pub struct RefreshRequest<'a> {
    pub product_id: &'a str,
    pub device_id: &'a str,
    pub lease: &'a str,
}

#[derive(Debug, Clone, Serialize)]
pub struct DeactivateRequest<'a> {
    pub product_id: &'a str,
    pub device_id: &'a str,
    pub lease: &'a str,
}

#[derive(Debug, Deserialize)]
struct LeaseResponse {
    lease: String,
}

const MAX_SUCCESS_RESPONSE_BYTES: usize = 64 * 1024;
const MAX_ERROR_RESPONSE_BYTES: usize = 4 * 1024;
const MAX_SIGNED_LEASE_BYTES: usize = 48 * 1024;

fn read_capped(mut reader: impl Read, limit: usize) -> Result<Vec<u8>> {
    let mut body = Vec::new();
    reader
        .by_ref()
        .take((limit as u64).saturating_add(1))
        .read_to_end(&mut body)
        .map_err(|e| LicenseError::Transport(format!("read response: {e}")))?;
    if body.len() > limit {
        return Err(LicenseError::Transport(format!(
            "response exceeds {limit} bytes"
        )));
    }
    Ok(body)
}

fn decode_lease_response(response: reqwest::blocking::Response, operation: &str) -> Result<String> {
    let body = read_capped(response, MAX_SUCCESS_RESPONSE_BYTES)?;
    let envelope: LeaseResponse = serde_json::from_slice(&body)
        .map_err(|e| LicenseError::Transport(format!("decode {operation} response: {e}")))?;
    if envelope.lease.is_empty() || envelope.lease.len() > MAX_SIGNED_LEASE_BYTES {
        return Err(LicenseError::Transport(format!(
            "{operation} response contains an invalid lease length"
        )));
    }
    Ok(envelope.lease)
}

pub trait LicenseTransport: Send + Sync {
    fn activate(&self, request: ActivateRequest<'_>) -> Result<String>;
    fn refresh(&self, request: RefreshRequest<'_>) -> Result<String>;
    fn deactivate(&self, request: DeactivateRequest<'_>) -> Result<()>;
}

pub struct HttpTransport {
    base_url: String,
    client: Client,
}

impl HttpTransport {
    pub fn new(base_url: &str, timeout: Duration, allow_insecure_localhost: bool) -> Result<Self> {
        let trimmed = base_url.trim_end_matches('/');
        let secure = trimmed.starts_with("https://");
        let local = allow_insecure_localhost
            && (trimmed.starts_with("http://127.0.0.1") || trimmed.starts_with("http://localhost"));
        if !secure && !local {
            return Err(LicenseError::Configuration(
                "server_url must use HTTPS; explicit HTTP is limited to localhost".into(),
            ));
        }
        let client = Client::builder()
            .timeout(timeout)
            .build()
            .map_err(|e| LicenseError::Configuration(format!("HTTP client: {e}")))?;
        Ok(Self {
            base_url: trimmed.into(),
            client,
        })
    }

    fn post<B: Serialize>(&self, path: &str, body: &B) -> Result<reqwest::blocking::Response> {
        let response = self
            .client
            .post(format!("{}{}", self.base_url, path))
            .json(body)
            .send()
            .map_err(|e| LicenseError::Transport(e.to_string()))?;
        if !response.status().is_success() {
            let status = response.status();
            let detail = read_capped(response, MAX_ERROR_RESPONSE_BYTES)
                .map(|body| String::from_utf8_lossy(&body).into_owned())
                .unwrap_or_default();
            return Err(LicenseError::Server(format!("HTTP {status}: {detail}")));
        }
        Ok(response)
    }
}

impl LicenseTransport for HttpTransport {
    fn activate(&self, request: ActivateRequest<'_>) -> Result<String> {
        decode_lease_response(self.post("/v1/client/activate", &request)?, "activation")
    }
    fn refresh(&self, request: RefreshRequest<'_>) -> Result<String> {
        decode_lease_response(self.post("/v1/client/refresh", &request)?, "refresh")
    }
    fn deactivate(&self, request: DeactivateRequest<'_>) -> Result<()> {
        self.post("/v1/client/deactivate", &request).map(|_| ())
    }
}

#[cfg(test)]
mod tests {
    use super::{read_capped, MAX_ERROR_RESPONSE_BYTES};
    use std::io::Cursor;

    #[test]
    fn capped_reader_rejects_oversized_response() {
        let body = vec![b'x'; MAX_ERROR_RESPONSE_BYTES + 1];
        assert!(read_capped(Cursor::new(body), MAX_ERROR_RESPONSE_BYTES).is_err());
    }
}
