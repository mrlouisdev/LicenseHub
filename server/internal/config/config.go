package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	licensecrypto "github.com/tabloy/keygate/internal/license"
)

type Config struct {
	Port        string
	Environment string
	BaseURL     string

	DatabaseURL string

	JWTSecret                 string
	LicenseSigningKey         string
	LicenseSigningKeyID       string
	LicenseLeaseTTL           string
	LicenseRetainedPublicKeys string

	StripeSecretKey     string
	StripeWebhookSecret string
	// StripeLivemode tells the webhook handler which environment to
	// trust. A mismatch between this flag and event.Livemode is a
	// configuration error or a forged delivery; either way the
	// handler must reject. Auto-derived from the secret key prefix
	// (sk_live_ vs sk_test_) unless STRIPE_LIVEMODE is set explicitly.
	StripeLivemode bool

	WebhookMaxAttempts    int
	WebhookRetryInterval  string
	WebhookHTTPTimeout    string
	QuotaWarningThreshold float64

	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string

	RedisURL string
	// MetricsToken protects Prometheus metrics in production. When empty,
	// /metrics is disabled outside development/staging instead of being public.
	MetricsToken string

	RateLimitAPI   int
	RateLimitAdmin int
	RateLimitAuth  int

	// Brute-force protection on /license/* — caps repeated bad license
	// keys per IP. In tests these defaults are too tight, so they're
	// configurable via env: BF_MAX_FAILS=5 / BF_LOCKOUT=30s / etc.
	BFMaxFails       int
	BFLockoutSeconds int

	AdminEmails []string

	// ─── Storage (release artifacts: R2 / S3 / S3-compatible) ───
	// All fields are optional. The storage subsystem is enabled iff
	// StorageBucket is non-empty and credentials are present. When disabled,
	// release endpoints return 503 — license/billing functions are unaffected.
	StorageEndpoint       string // e.g. https://<account>.r2.cloudflarestorage.com (empty = AWS S3)
	StorageRegion         string // R2 uses "auto"; AWS S3 uses real region
	StorageBucket         string
	StorageAccessKey      string
	StorageSecretKey      string
	StoragePublicURL      string // optional CDN URL prefix for public reads (not used for license-gated downloads)
	StorageForcePathStyle bool   // true for MinIO and some self-hosted S3 gateways

	// Presigned URL TTLs.
	StorageUploadTTL   string // default "1h"
	StorageDownloadTTL string // default "10m"

	// ReleaseKeyEncryptionKey is a 64-char hex string (32 bytes) used as the
	// AES-256-GCM master key for encrypting product release-signing private
	// keys at rest. Required when storage is enabled.
	//
	// Operational notes:
	//   - Generate via: openssl rand -hex 32
	//   - Rotation requires re-encrypting all release_signing_keys rows.
	//     There is no automatic migration on key change — the operator must
	//     run a re-encryption script, otherwise existing keys become
	//     undecryptable and signing fails.
	//   - Losing this key permanently locks all signed releases.
	ReleaseKeyEncryptionKey string

	// MaxReleaseSignSize caps the largest artifact we will sign server-side.
	// Pure Ed25519 requires the full message in memory; 500 MB is a
	// reasonable default that doesn't OOM modest VMs. Larger artifacts
	// must use unsigned mode (Phase 3 will add streaming via tempfile).
	MaxReleaseSignSize int64
}

func Load() (*Config, error) {
	_ = godotenv.Load()
	databaseURL, err := envOrFile("DATABASE_URL")
	if err != nil {
		return nil, err
	}
	jwtSecret, err := envOrFile("JWT_SECRET")
	if err != nil {
		return nil, err
	}
	licenseSigningKey, err := envOrFile("LICENSE_SIGNING_KEY")
	if err != nil {
		return nil, err
	}
	stripeSecretKey, err := envOrFile("STRIPE_SECRET_KEY")
	if err != nil {
		return nil, err
	}
	metricsToken, err := envOrFile("METRICS_TOKEN")
	if err != nil {
		return nil, err
	}
	smtpPassword, err := envOrFile("SMTP_PASSWORD")
	if err != nil {
		return nil, err
	}
	storageAccessKey, err := envOrFile("STORAGE_ACCESS_KEY")
	if err != nil {
		return nil, err
	}
	storageSecretKey, err := envOrFile("STORAGE_SECRET_KEY")
	if err != nil {
		return nil, err
	}
	releaseKeyEncryptionKey, err := envOrFile("RELEASE_KEY_ENCRYPTION_KEY")
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Port:        envOr("PORT", "9000"),
		Environment: envOr("ENVIRONMENT", "development"),
		BaseURL:     envOr("BASE_URL", "http://localhost:9000"),

		DatabaseURL: databaseURL,

		JWTSecret:                 jwtSecret,
		LicenseSigningKey:         licenseSigningKey,
		LicenseSigningKeyID:       envOr("LICENSE_SIGNING_KEY_ID", "v1"),
		LicenseLeaseTTL:           envOr("LICENSE_LEASE_TTL", "72h"),
		LicenseRetainedPublicKeys: envOr("LICENSE_RETAINED_PUBLIC_KEYS", "{}"),

		StripeSecretKey:     stripeSecretKey,
		StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
	}

	envVal, envSet := os.LookupEnv("STRIPE_LIVEMODE")
	cfg.StripeLivemode = deriveLivemode(envVal, envSet, cfg.StripeSecretKey)

	cfg.RedisURL = os.Getenv("REDIS_URL")
	cfg.MetricsToken = metricsToken

	cfg.SMTPHost = os.Getenv("SMTP_HOST")
	cfg.SMTPPort = envOr("SMTP_PORT", "587")
	cfg.SMTPUsername = os.Getenv("SMTP_USERNAME")
	cfg.SMTPPassword = smtpPassword
	cfg.SMTPFrom = os.Getenv("SMTP_FROM")

	cfg.RateLimitAPI = envIntOr("RATE_LIMIT_API", 60)
	cfg.RateLimitAdmin = envIntOr("RATE_LIMIT_ADMIN", 120)
	cfg.RateLimitAuth = envIntOr("RATE_LIMIT_AUTH", 20)
	cfg.BFMaxFails = envIntOr("BF_MAX_FAILS", 5)
	cfg.BFLockoutSeconds = envIntOr("BF_LOCKOUT_SECONDS", 30)

	cfg.WebhookMaxAttempts = envIntOr("WEBHOOK_MAX_ATTEMPTS", 5)
	cfg.WebhookRetryInterval = envOr("WEBHOOK_RETRY_INTERVAL", "30s")
	cfg.WebhookHTTPTimeout = envOr("WEBHOOK_HTTP_TIMEOUT", "10s")
	cfg.QuotaWarningThreshold = envFloatOr("QUOTA_WARNING_THRESHOLD", 0.8)

	if admins := os.Getenv("ADMIN_EMAILS"); admins != "" {
		for _, e := range strings.Split(admins, ",") {
			cfg.AdminEmails = append(cfg.AdminEmails, strings.TrimSpace(e))
		}
	}

	cfg.StorageEndpoint = os.Getenv("STORAGE_ENDPOINT")
	cfg.StorageRegion = envOr("STORAGE_REGION", "auto")
	cfg.StorageBucket = os.Getenv("STORAGE_BUCKET")
	cfg.StorageAccessKey = storageAccessKey
	cfg.StorageSecretKey = storageSecretKey
	cfg.StoragePublicURL = os.Getenv("STORAGE_PUBLIC_URL")
	cfg.StorageForcePathStyle = strings.EqualFold(os.Getenv("STORAGE_FORCE_PATH_STYLE"), "true")
	cfg.StorageUploadTTL = envOr("STORAGE_UPLOAD_TTL", "1h")
	cfg.StorageDownloadTTL = envOr("STORAGE_DOWNLOAD_TTL", "10m")
	cfg.ReleaseKeyEncryptionKey = releaseKeyEncryptionKey
	cfg.MaxReleaseSignSize = int64(envIntOr("MAX_RELEASE_SIGN_SIZE_MB", 500)) * 1024 * 1024

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}
	if cfg.LicenseSigningKey == "" {
		return nil, fmt.Errorf("LICENSE_SIGNING_KEY is required")
	}

	return cfg, nil
}

func (c *Config) normalizedEnv() string {
	return strings.ToLower(strings.TrimSpace(c.Environment))
}

func (c *Config) IsProduction() bool { return c.normalizedEnv() == "production" }

func (c *Config) IsDevLoginAllowed() bool { return c.normalizedEnv() == "development" }

// IsAdminEmail checks if an email is in the ADMIN_EMAILS list.
// Used for backward compatibility and initial setup bootstrap.
// In normal operation, admin status is determined by the user's role in the database.
func (c *Config) IsAdminEmail(email string) bool {
	for _, e := range c.AdminEmails {
		if strings.EqualFold(e, email) {
			return true
		}
	}
	return false
}

// deriveLivemode decides whether the server should treat Stripe
// events as live (real money) or test. Order of precedence:
//
//  1. STRIPE_LIVEMODE env (case-insensitive "true" or "1") wins.
//  2. sk_live_… / rk_live_… secret key prefix → true.
//  3. sk_test_… / rk_test_… → false.
//  4. anything else (including unset) → false. Safe default:
//     operators must opt INTO live mode rather than fall into it
//     by accident. Mismatched events get a 400 in the webhook
//     handler, so this default minimises the blast radius of a
//     half-configured deployment.
//
// Pulled out as a free function so it can be unit-tested without
// the full Load() side-effects (godotenv, ADMIN_EMAILS parsing, etc).
func deriveLivemode(envVal string, envSet bool, secretKey string) bool {
	if envSet {
		return strings.EqualFold(envVal, "true") || envVal == "1"
	}
	return strings.HasPrefix(secretKey, "sk_live_") ||
		strings.HasPrefix(secretKey, "rk_live_")
}

// IsStorageEnabled reports whether the storage subsystem (release artifacts)
// has the minimum required configuration. Endpoint/region/path-style are
// optional — only bucket+credentials are mandatory.
func (c *Config) IsStorageEnabled() bool {
	return c.StorageBucket != "" &&
		c.StorageAccessKey != "" &&
		c.StorageSecretKey != ""
}

// IsMasterEncryptionKeyConfigured reports whether the operator supplied the
// AES-256 master key that's used to derive subkeys for:
//   - license_key at-rest encryption
//   - release artifact signing private keys
//
// These two features are independently useful: a deployment that never
// distributes binaries can still benefit from license-key encryption.
// We therefore treat the master key as orthogonal to storage.
func (c *Config) IsMasterEncryptionKeyConfigured() bool {
	return len(c.ReleaseKeyEncryptionKey) == 64
}

// ValidateSecurityDefaults checks for common misconfigurations that could
// lead to security vulnerabilities in production deployments.
// Returns a list of warnings (non-fatal) and errors (fatal).
func (c *Config) ValidateSecurityDefaults() (warnings []string, fatal []string) {
	// Validate environment value
	env := strings.ToLower(strings.TrimSpace(c.Environment))
	switch env {
	case "development", "staging", "production":
		// valid
	default:
		fatal = append(fatal, "ENVIRONMENT must be 'development', 'staging', or 'production', got: '"+c.Environment+"'")
	}

	// Fatal: JWT secret too short
	if len(c.JWTSecret) < 32 {
		fatal = append(fatal, "JWT_SECRET must be at least 32 characters")
	}
	// LICENSE_SIGNING_KEY must be a 32-byte ed25519 seed, hex-encoded
	// (64 chars). It signs the offline-verifiable license token that
	// SDKs hand to desktop clients. Ed25519 + hex-encoded seed is the
	// industry-standard format for env-var-style key distribution.
	if seed, err := hex.DecodeString(strings.TrimSpace(c.LicenseSigningKey)); err != nil || len(seed) != 32 {
		fatal = append(fatal,
			"LICENSE_SIGNING_KEY must be a 32-byte ed25519 seed in hex (64 chars); generate with 'openssl rand -hex 32'")
	}
	keyID := c.LicenseSigningKeyID
	if keyID == "" {
		keyID = "v1"
	}
	if !validSigningKeyID(keyID) {
		fatal = append(fatal, "LICENSE_SIGNING_KEY_ID must be 1-64 characters using only letters, digits, '.', '_' or '-'")
	}
	retained, retainedErr := licensecrypto.ParseRetainedPublicKeys(c.LicenseRetainedPublicKeys)
	if retainedErr != nil {
		fatal = append(fatal, "LICENSE_RETAINED_PUBLIC_KEYS: "+retainedErr.Error())
	} else if _, duplicatesActive := retained[keyID]; duplicatesActive {
		fatal = append(fatal, "LICENSE_RETAINED_PUBLIC_KEYS must not contain active LICENSE_SIGNING_KEY_ID; the active public key is included automatically")
	}
	leaseTTLRaw := c.LicenseLeaseTTL
	if leaseTTLRaw == "" {
		leaseTTLRaw = "72h"
	}
	leaseTTL, err := time.ParseDuration(leaseTTLRaw)
	if err != nil {
		fatal = append(fatal, "LICENSE_LEASE_TTL must be a valid Go duration such as '72h'")
	} else if leaseTTL < 5*time.Minute || leaseTTL > 30*24*time.Hour {
		fatal = append(fatal, "LICENSE_LEASE_TTL must be between 5m and 720h")
	}

	if c.IsProduction() {
		// Must have at least one admin
		if len(c.AdminEmails) == 0 {
			warnings = append(warnings, "SECURITY: ADMIN_EMAILS is empty — no one can access the admin panel")
		}
		if strings.TrimSpace(c.RedisURL) == "" {
			fatal = append(fatal, "REDIS_URL is required in production for persistent distributed abuse controls")
		}
		if strings.TrimSpace(c.SMTPHost) == "" || strings.TrimSpace(c.SMTPFrom) == "" {
			fatal = append(fatal, "SMTP_HOST and SMTP_FROM are required in production so OTP authentication cannot fall back or fail silently")
		}
	}

	if c.IsDevLoginAllowed() {
		warnings = append(warnings, "SECURITY: dev-login is enabled (ENVIRONMENT=development) — do NOT use in production")
	}
	if c.MetricsToken != "" && len(c.MetricsToken) < 32 {
		fatal = append(fatal, "METRICS_TOKEN must be at least 32 characters when configured")
	}

	// Storage: validate that partial config doesn't silently disable releases.
	// If any storage field is set, all required fields must be set.
	storageFieldsSet := c.StorageBucket != "" ||
		c.StorageAccessKey != "" ||
		c.StorageSecretKey != "" ||
		c.StorageEndpoint != ""
	if storageFieldsSet && !c.IsStorageEnabled() {
		fatal = append(fatal, "STORAGE_*: partial config detected — STORAGE_BUCKET, STORAGE_ACCESS_KEY, and STORAGE_SECRET_KEY must all be set together (or all empty to disable releases)")
	}

	// Release signing master key: required iff storage is enabled. Length
	// must be exactly 64 hex chars (32 bytes for AES-256). We don't accept
	// the "fallback to a derived key" mode — operators must explicitly own
	// this secret because losing it means losing the ability to verify
	// past signatures.
	// Master encryption key validation: required when storage is enabled
	// (release signing needs it) AND validated even when only set without
	// storage (license-key encryption uses it independently).
	switch {
	case c.IsStorageEnabled() && c.ReleaseKeyEncryptionKey == "":
		fatal = append(fatal, "RELEASE_KEY_ENCRYPTION_KEY is required when storage is enabled — generate via: openssl rand -hex 32")
	case c.IsProduction() && c.ReleaseKeyEncryptionKey == "":
		fatal = append(fatal, "RELEASE_KEY_ENCRYPTION_KEY is required in production so license keys are encrypted at rest")
	case c.ReleaseKeyEncryptionKey == "":
		// Not provided and not required — license encryption stays disabled,
		// release signing isn't applicable. Surfaced as a warning since this
		// disables a security feature.
		warnings = append(warnings, "SECURITY: RELEASE_KEY_ENCRYPTION_KEY is not set — license keys are stored in plaintext")
	case len(c.ReleaseKeyEncryptionKey) != 64:
		fatal = append(fatal, "RELEASE_KEY_ENCRYPTION_KEY must be exactly 64 hex chars (32 bytes for AES-256)")
	default:
		if _, err := hex.DecodeString(c.ReleaseKeyEncryptionKey); err != nil {
			fatal = append(fatal, "RELEASE_KEY_ENCRYPTION_KEY is not valid hex: "+err.Error())
		}
	}

	return
}

func validSigningKeyID(v string) bool {
	if len(v) < 1 || len(v) > 64 {
		return false
	}
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envOrFile accepts either KEY or KEY_FILE. The file variant keeps values out
// of container metadata while remaining compatible with direct local config.
func envOrFile(key string) (string, error) {
	value, valueSet := os.LookupEnv(key)
	path, fileSet := os.LookupEnv(key + "_FILE")
	if valueSet && fileSet {
		return "", fmt.Errorf("%s and %s_FILE are mutually exclusive", key, key)
	}
	if !fileSet {
		return value, nil
	}
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s_FILE must be an absolute path", key)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("read %s_FILE: %w", key, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%s_FILE must reference a regular non-symlink file", key)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o022 != 0 {
		return "", fmt.Errorf("%s_FILE must not be group/world writable", key)
	}
	if info.Size() > 64*1024 {
		return "", fmt.Errorf("%s_FILE exceeds 64 KiB", key)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s_FILE: %w", key, err)
	}
	return strings.TrimRight(string(contents), "\r\n"), nil
}

func envIntOr(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envFloatOr(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}
