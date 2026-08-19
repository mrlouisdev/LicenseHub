use std::time::{SystemTime, UNIX_EPOCH};

use crate::error::{LicenseError, Result};

pub trait Clock: Send + Sync {
    fn unix_seconds(&self) -> Result<i64>;
}

#[derive(Debug, Default)]
pub struct SystemClock;

impl Clock for SystemClock {
    fn unix_seconds(&self) -> Result<i64> {
        let duration = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map_err(|e| LicenseError::Internal(format!("system time precedes Unix epoch: {e}")))?;
        i64::try_from(duration.as_secs())
            .map_err(|_| LicenseError::Internal("system time overflow".into()))
    }
}
