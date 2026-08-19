use thiserror::Error;

#[derive(Debug, Error)]
pub enum LicenseError {
    #[error("invalid configuration: {0}")]
    Configuration(String),
    #[error("transport failed: {0}")]
    Transport(String),
    #[error("server rejected request: {0}")]
    Server(String),
    #[error("invalid lease: {0}")]
    InvalidLease(String),
    #[error("lease signature is invalid")]
    InvalidSignature,
    #[error("lease belongs to product '{actual}', expected '{expected}'")]
    ProductMismatch { expected: String, actual: String },
    #[error("lease belongs to another device")]
    DeviceMismatch,
    #[error("lease expired at {0}")]
    Expired(i64),
    #[error("system clock moved backwards (now={now}, last_seen={last_seen})")]
    ClockRollback { now: i64, last_seen: i64 },
    #[error("entitlement '{0}' is not granted")]
    EntitlementMissing(String),
    #[error("secure storage failed: {0}")]
    Storage(String),
    #[error("not activated")]
    NotActivated,
    #[error("invalid argument: {0}")]
    InvalidArgument(String),
    #[error("internal error: {0}")]
    Internal(String),
}

impl LicenseError {
    pub fn code(&self) -> i32 {
        match self {
            Self::Configuration(_) => 10,
            Self::Transport(_) => 20,
            Self::Server(_) => 21,
            Self::InvalidLease(_) => 30,
            Self::InvalidSignature => 31,
            Self::ProductMismatch { .. } => 32,
            Self::DeviceMismatch => 33,
            Self::Expired(_) => 34,
            Self::ClockRollback { .. } => 35,
            Self::EntitlementMissing(_) => 36,
            Self::Storage(_) => 40,
            Self::NotActivated => 41,
            Self::InvalidArgument(_) => 50,
            Self::Internal(_) => 99,
        }
    }
}

pub type Result<T> = std::result::Result<T, LicenseError>;
