use std::{
    collections::{BTreeSet, HashMap, VecDeque},
    sync::{Arc, Mutex},
};

use base64::{
    engine::general_purpose::{STANDARD, URL_SAFE_NO_PAD},
    Engine as _,
};
use ed25519_dalek::{Signer, SigningKey};

use crate::{
    client::{ClientConfig, LicenseClient, LicenseState},
    clock::Clock,
    error::{LicenseError, Result},
    lease::{LeaseClaims, LEASE_VERSION},
    store::testing::MemoryStore,
    transport::{ActivateRequest, DeactivateRequest, LicenseTransport, RefreshRequest},
};

#[derive(Default)]
struct MockTransport {
    activate: Mutex<VecDeque<Result<String>>>,
    refresh: Mutex<VecDeque<Result<String>>>,
}

impl LicenseTransport for MockTransport {
    fn activate(&self, _: ActivateRequest<'_>) -> Result<String> {
        self.activate.lock().unwrap().pop_front().unwrap()
    }
    fn refresh(&self, _: RefreshRequest<'_>) -> Result<String> {
        self.refresh.lock().unwrap().pop_front().unwrap()
    }
    fn deactivate(&self, _: DeactivateRequest<'_>) -> Result<()> {
        Ok(())
    }
}

struct MockClock(Mutex<i64>);
impl Clock for MockClock {
    fn unix_seconds(&self) -> Result<i64> {
        Ok(*self.0.lock().unwrap())
    }
}

fn config(signing: &SigningKey) -> ClientConfig {
    ClientConfig {
        product_id: "app_a".into(),
        public_keys: HashMap::from([(
            "primary".into(),
            STANDARD.encode(signing.verifying_key().to_bytes()),
        )]),
        clock_rollback_tolerance_seconds: 5,
    }
}

fn rotating_config(current: &SigningKey, next: &SigningKey) -> ClientConfig {
    ClientConfig {
        product_id: "app_a".into(),
        public_keys: HashMap::from([
            (
                "current".into(),
                STANDARD.encode(current.verifying_key().to_bytes()),
            ),
            (
                "next".into(),
                STANDARD.encode(next.verifying_key().to_bytes()),
            ),
        ]),
        clock_rollback_tolerance_seconds: 5,
    }
}

fn token(
    signing: &SigningKey,
    kid: &str,
    product: &str,
    device: &str,
    iat: i64,
    exp: i64,
) -> String {
    let claims = LeaseClaims {
        version: LEASE_VERSION,
        kid: kid.into(),
        license_id: "lic_1".into(),
        product_id: product.into(),
        device_id: device.into(),
        entitlements: BTreeSet::from(["pro".into()]),
        iat,
        exp,
    };
    let payload = URL_SAFE_NO_PAD.encode(serde_json::to_vec(&claims).unwrap());
    let signature = signing.sign(payload.as_bytes());
    format!("{payload}.{}", URL_SAFE_NO_PAD.encode(signature.to_bytes()))
}

fn harness(
    now: i64,
) -> (
    LicenseClient,
    Arc<MockTransport>,
    Arc<MockClock>,
    SigningKey,
) {
    let signing = SigningKey::from_bytes(&[7u8; 32]);
    let transport = Arc::new(MockTransport::default());
    let clock = Arc::new(MockClock(Mutex::new(now)));
    let client = LicenseClient::initialize(
        config(&signing),
        transport.clone(),
        Arc::new(MemoryStore::default()),
        clock.clone(),
    )
    .unwrap();
    (client, transport, clock, signing)
}

#[test]
fn rejects_oversized_or_malformed_security_config() {
    let signing = SigningKey::from_bytes(&[7u8; 32]);
    let transport = Arc::new(MockTransport::default());
    let store = Arc::new(MemoryStore::default());
    let clock = Arc::new(MockClock(Mutex::new(1_000)));

    let mut bad_tolerance = config(&signing);
    bad_tolerance.clock_rollback_tolerance_seconds = 86_400;
    assert!(LicenseClient::initialize(
        bad_tolerance,
        transport.clone(),
        store.clone(),
        clock.clone(),
    )
    .is_err());

    let mut bad_key_id = config(&signing);
    bad_key_id.public_keys = HashMap::from([(
        "bad key id".into(),
        STANDARD.encode(signing.verifying_key().to_bytes()),
    )]);
    assert!(LicenseClient::initialize(bad_key_id, transport, store, clock).is_err());
}

#[test]
fn rejects_oversized_signed_lease_before_parsing() {
    use crate::lease::{verify_token, MAX_SIGNED_LEASE_BYTES};
    let signing = SigningKey::from_bytes(&[7u8; 32]);
    let keys = HashMap::from([("primary".into(), signing.verifying_key())]);
    let oversized = "x".repeat(MAX_SIGNED_LEASE_BYTES + 1);
    assert!(matches!(
        verify_token(&oversized, &keys),
        Err(LicenseError::InvalidLease(_))
    ));
}

#[test]
fn activate_verifies_signature_binding_and_entitlement() {
    let (mut client, transport, _, signing) = harness(1_000);
    let signed = token(&signing, "primary", "app_a", client.device_id(), 900, 1_100);
    transport.activate.lock().unwrap().push_back(Ok(signed));
    assert_eq!(
        client.activate("KEY-1").unwrap().state,
        LicenseState::Active
    );
    client.require_entitlement("pro").unwrap();
    assert!(matches!(
        client.require_entitlement("enterprise"),
        Err(LicenseError::EntitlementMissing(_))
    ));
}

#[test]
fn rejects_lease_for_another_device() {
    let (mut client, transport, _, signing) = harness(1_000);
    transport.activate.lock().unwrap().push_back(Ok(token(
        &signing,
        "primary",
        "app_a",
        "dev_other",
        900,
        1_100,
    )));
    assert!(matches!(
        client.activate("KEY-1"),
        Err(LicenseError::DeviceMismatch)
    ));
}

#[test]
fn rejects_tampered_signature() {
    let (mut client, transport, _, signing) = harness(1_000);
    let mut signed = token(&signing, "primary", "app_a", client.device_id(), 900, 1_100);
    let signature_start = signed.find('.').unwrap() + 1;
    let replacement = if signed.as_bytes()[signature_start] == b'A' {
        "B"
    } else {
        "A"
    };
    signed.replace_range(signature_start..signature_start + 1, replacement);
    transport.activate.lock().unwrap().push_back(Ok(signed));
    assert!(matches!(
        client.activate("KEY-1"),
        Err(LicenseError::InvalidSignature)
    ));
}

#[test]
fn token_exp_controls_72_hour_offline_window() {
    let (mut client, transport, clock, signing) = harness(1_000);
    transport.activate.lock().unwrap().push_back(Ok(token(
        &signing,
        "primary",
        "app_a",
        client.device_id(),
        1_000,
        1_000 + 72 * 3600,
    )));
    client.activate("KEY-1").unwrap();
    *clock.0.lock().unwrap() = 1_000 + 72 * 3600 - 1;
    assert_eq!(client.status().unwrap().state, LicenseState::Active);
    *clock.0.lock().unwrap() = 1_000 + 72 * 3600;
    assert_eq!(client.status().unwrap().state, LicenseState::Expired);
}

#[test]
fn detects_clock_rollback_beyond_tolerance() {
    let (mut client, transport, clock, signing) = harness(1_000);
    transport.activate.lock().unwrap().push_back(Ok(token(
        &signing,
        "primary",
        "app_a",
        client.device_id(),
        900,
        2_000,
    )));
    client.activate("KEY-1").unwrap();
    *clock.0.lock().unwrap() = 1_100;
    assert_eq!(client.status().unwrap().state, LicenseState::Active);
    *clock.0.lock().unwrap() = 1_000;
    assert_eq!(client.status().unwrap().state, LicenseState::ClockRollback);
}

#[test]
fn refresh_replaces_verified_lease() {
    let (mut client, transport, _, signing) = harness(1_000);
    transport.activate.lock().unwrap().push_back(Ok(token(
        &signing,
        "primary",
        "app_a",
        client.device_id(),
        900,
        1_100,
    )));
    client.activate("KEY-1").unwrap();
    transport.refresh.lock().unwrap().push_back(Ok(token(
        &signing,
        "primary",
        "app_a",
        client.device_id(),
        1_000,
        2_000,
    )));
    assert_eq!(client.refresh().unwrap().expires_at, Some(2_000));
}

#[test]
fn staged_pinned_keyring_survives_server_signer_rotation() {
    let current = SigningKey::from_bytes(&[7u8; 32]);
    let next = SigningKey::from_bytes(&[8u8; 32]);
    let transport = Arc::new(MockTransport::default());
    let clock = Arc::new(MockClock(Mutex::new(1_000)));
    let config = rotating_config(&current, &next);
    assert_eq!(config.pinned_key_ids(), vec!["current", "next"]);
    let mut client = LicenseClient::initialize(
        config,
        transport.clone(),
        Arc::new(MemoryStore::default()),
        clock,
    )
    .unwrap();

    transport.activate.lock().unwrap().push_back(Ok(token(
        &current,
        "current",
        "app_a",
        client.device_id(),
        900,
        1_100,
    )));
    client.activate("KEY-1").unwrap();

    transport.refresh.lock().unwrap().push_back(Ok(token(
        &next,
        "next",
        "app_a",
        client.device_id(),
        1_000,
        2_000,
    )));
    assert_eq!(client.refresh().unwrap().expires_at, Some(2_000));
}

#[test]
fn unexpected_rotation_key_is_rejected_instead_of_network_trusted() {
    let current = SigningKey::from_bytes(&[7u8; 32]);
    let untrusted = SigningKey::from_bytes(&[9u8; 32]);
    let transport = Arc::new(MockTransport::default());
    let clock = Arc::new(MockClock(Mutex::new(1_000)));
    let mut client = LicenseClient::initialize(
        config(&current),
        transport.clone(),
        Arc::new(MemoryStore::default()),
        clock,
    )
    .unwrap();
    transport.activate.lock().unwrap().push_back(Ok(token(
        &untrusted,
        "network-announced",
        "app_a",
        client.device_id(),
        900,
        1_100,
    )));
    assert!(matches!(
        client.activate("KEY-1"),
        Err(LicenseError::InvalidLease(message)) if message.contains("unknown signing key")
    ));
}

#[test]
fn device_identity_and_lease_survive_restart() {
    let signing = SigningKey::from_bytes(&[7u8; 32]);
    let store = Arc::new(MemoryStore::default());
    let transport = Arc::new(MockTransport::default());
    let clock = Arc::new(MockClock(Mutex::new(1_000)));
    let mut first = LicenseClient::initialize(
        config(&signing),
        transport.clone(),
        store.clone(),
        clock.clone(),
    )
    .unwrap();
    let device = first.device_id().to_owned();
    transport
        .activate
        .lock()
        .unwrap()
        .push_back(Ok(token(&signing, "primary", "app_a", &device, 900, 2_000)));
    first.activate("KEY-1").unwrap();
    drop(first);
    let mut restarted =
        LicenseClient::initialize(config(&signing), transport, store, clock).unwrap();
    assert_eq!(restarted.device_id(), device);
    assert_eq!(restarted.status().unwrap().state, LicenseState::Active);
}
