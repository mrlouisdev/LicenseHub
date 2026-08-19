#![forbid(unsafe_op_in_unsafe_fn)]

pub mod client;
pub mod clock;
pub mod error;
pub mod ffi;
pub mod lease;
pub mod store;
pub mod transport;

pub use client::{ClientConfig, LicenseClient, LicenseState, LicenseStatus};
pub use error::{LicenseError, Result};
pub use lease::{LeaseClaims, VerifiedLease};

#[cfg(test)]
mod tests;
