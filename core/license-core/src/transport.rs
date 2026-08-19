use std::time::Duration;

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
            let detail = response.text().unwrap_or_default();
            return Err(LicenseError::Server(format!("HTTP {status}: {detail}")));
        }
        Ok(response)
    }
}

impl LicenseTransport for HttpTransport {
    fn activate(&self, request: ActivateRequest<'_>) -> Result<String> {
        self.post("/v1/client/activate", &request)?
            .json::<LeaseResponse>()
            .map(|r| r.lease)
            .map_err(|e| LicenseError::Transport(format!("decode activation response: {e}")))
    }
    fn refresh(&self, request: RefreshRequest<'_>) -> Result<String> {
        self.post("/v1/client/refresh", &request)?
            .json::<LeaseResponse>()
            .map(|r| r.lease)
            .map_err(|e| LicenseError::Transport(format!("decode refresh response: {e}")))
    }
    fn deactivate(&self, request: DeactivateRequest<'_>) -> Result<()> {
        self.post("/v1/client/deactivate", &request).map(|_| ())
    }
}
