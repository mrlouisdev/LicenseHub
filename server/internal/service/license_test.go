package service

import (
	"bytes"
	"crypto/ed25519"
	"reflect"
	"testing"
	"time"

	"github.com/tabloy/keygate/internal/license"
	"github.com/tabloy/keygate/internal/model"
)

func TestAssertUsable(t *testing.T) {
	svc := &LicenseService{}

	now := time.Now()
	future := now.Add(24 * time.Hour)
	past := now.Add(-24 * time.Hour)
	wayPast := now.Add(-30 * 24 * time.Hour)

	tests := []struct {
		name    string
		license *model.License
		wantErr bool
		errCode string
	}{
		{
			name:    "active license with future expiry",
			license: &model.License{Status: model.StatusActive, ValidUntil: &future, Plan: &model.Plan{GraceDays: 7}},
			wantErr: false,
		},
		{
			name:    "active license no expiry",
			license: &model.License{Status: model.StatusActive, Plan: &model.Plan{GraceDays: 7}},
			wantErr: false,
		},
		{
			name:    "active license recently expired within grace",
			license: &model.License{Status: model.StatusActive, ValidUntil: &past, Plan: &model.Plan{GraceDays: 7}},
			wantErr: false,
		},
		{
			name:    "active license expired beyond grace",
			license: &model.License{Status: model.StatusActive, ValidUntil: &wayPast, Plan: &model.Plan{GraceDays: 7}},
			wantErr: true,
			errCode: "LICENSE_EXPIRED",
		},
		{
			name:    "trialing license",
			license: &model.License{Status: model.StatusTrialing, Plan: &model.Plan{GraceDays: 7}},
			wantErr: false,
		},
		{
			name:    "canceled license within paid period",
			license: &model.License{Status: model.StatusCanceled, ValidUntil: &future},
			wantErr: false,
		},
		{
			name:    "canceled license past paid period",
			license: &model.License{Status: model.StatusCanceled, ValidUntil: &past},
			wantErr: true,
			errCode: "LICENSE_CANCELED",
		},
		{
			name:    "suspended license",
			license: &model.License{Status: model.StatusSuspended},
			wantErr: true,
			errCode: "LICENSE_SUSPENDED",
		},
		{
			name:    "revoked license",
			license: &model.License{Status: model.StatusRevoked},
			wantErr: true,
			errCode: "LICENSE_REVOKED",
		},
		{
			name:    "expired license",
			license: &model.License{Status: model.StatusExpired},
			wantErr: true,
			errCode: "LICENSE_EXPIRED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.assertUsable(tt.license)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				// Check error contains expected code
				if tt.errCode != "" && !containsCode(err, tt.errCode) {
					t.Fatalf("expected error code %s, got %v", tt.errCode, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
		})
	}
}

func containsCode(err error, code string) bool {
	return err != nil && containsStr(err.Error(), code)
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestMaxActivations(t *testing.T) {
	svc := &LicenseService{}

	tests := []struct {
		name string
		lic  *model.License
		want int
	}{
		{"with plan", &model.License{Plan: &model.Plan{MaxActivations: 5}}, 5},
		{"without plan", &model.License{}, 1},
		{"nil plan", &model.License{Plan: nil}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.maxActivations(tt.lic)
			if got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSignTokenUsesConfiguredLeaseAndKeyID(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	priv := ed25519.NewKeyFromSeed(seed)
	svc := &LicenseService{signingKey: priv, signingKeyID: "lease-2026-01", leaseTTL: 72 * time.Hour}
	lic := &model.License{ID: "lic", ProductID: "prod", PlanID: "plan", Status: model.StatusActive}

	raw, err := svc.signToken(lic, "device")
	if err != nil {
		t.Fatalf("signToken: %v", err)
	}
	parsed, err := license.VerifyWithKeyRing(raw, map[string]ed25519.PublicKey{
		"lease-2026-01": priv.Public().(ed25519.PublicKey),
	})
	if err != nil {
		t.Fatalf("VerifyWithKeyRing: %v", err)
	}
	if parsed.KeyID != "lease-2026-01" {
		t.Fatalf("kid = %q", parsed.KeyID)
	}
	if got := time.Duration(parsed.ExpiresAt-parsed.IssuedAt) * time.Second; got != 72*time.Hour {
		t.Fatalf("lease ttl = %v", got)
	}
}

func TestOldKeyLeaseRenewsToActiveKid(t *testing.T) {
	oldKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, ed25519.SeedSize))
	activeKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{2}, ed25519.SeedSize))
	svc := &LicenseService{
		signingKey: activeKey, signingKeyID: "active-2026", leaseTTL: 72 * time.Hour,
		verificationKeys: map[string]ed25519.PublicKey{
			"old-2025":    oldKey.Public().(ed25519.PublicKey),
			"active-2026": activeKey.Public().(ed25519.PublicKey),
		},
	}
	oldLease, err := license.Sign(&license.VerifyToken{
		Version: 1, KeyID: "old-2025", LicenseID: "lic", ProductID: "prod",
		Identifier: "device", IssuedAt: time.Now().Add(-time.Hour).Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(), Entitlements: []string{"pro"},
	}, oldKey)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := svc.verifyLeaseBinding(LeaseInput{Lease: oldLease, ProductID: "prod", Identifier: "device"})
	if err != nil || claims.KeyID != "old-2025" {
		t.Fatalf("old lease rejected: claims=%+v err=%v", claims, err)
	}

	renewed, err := svc.signToken(&model.License{
		ID: "lic", ProductID: "prod", Status: model.StatusActive,
		Plan: &model.Plan{Entitlements: []*model.Entitlement{{Feature: "pro", ValueType: "bool", Value: "true"}}},
	}, "device")
	if err != nil {
		t.Fatal(err)
	}
	newClaims, err := license.VerifyWithKeyRing(renewed, svc.verificationKeys)
	if err != nil || newClaims.KeyID != "active-2026" {
		t.Fatalf("renewed lease did not use active kid: claims=%+v err=%v", newClaims, err)
	}

	unknownKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{3}, ed25519.SeedSize))
	unknownLease, _ := license.Sign(&license.VerifyToken{
		Version: 1, KeyID: "unknown", LicenseID: "lic", ProductID: "prod",
		Identifier: "device", IssuedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}, unknownKey)
	if _, err := svc.verifyLeaseBinding(LeaseInput{Lease: unknownLease, ProductID: "prod", Identifier: "device"}); err == nil {
		t.Fatal("expected unknown kid to fail")
	}
}

func TestGraceDays(t *testing.T) {
	svc := &LicenseService{}

	tests := []struct {
		name string
		lic  *model.License
		want int
	}{
		{"with plan", &model.License{Plan: &model.Plan{GraceDays: 14}}, 14},
		{"without plan", &model.License{}, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.graceDays(tt.lic)
			if got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestEntitlements(t *testing.T) {
	svc := &LicenseService{}

	lic := &model.License{
		Plan: &model.Plan{
			Entitlements: []*model.Entitlement{
				{Feature: "export", ValueType: "bool", Value: "true"},
				{Feature: "sso", ValueType: "bool", Value: "false"},
				{Feature: "max_users", ValueType: "int", Value: "50"},
				{Feature: "sla", ValueType: "string", Value: "99.9%"},
			},
		},
	}

	features := svc.entitlements(lic)

	if features["export"] != true {
		t.Error("export should be true")
	}
	if features["sso"] != false {
		t.Error("sso should be false")
	}
	if features["max_users"] != "50" {
		t.Errorf("max_users should be '50', got %v", features["max_users"])
	}
	if features["sla"] != "99.9%" {
		t.Errorf("sla should be '99.9%%', got %v", features["sla"])
	}

	nilLic := &model.License{}
	emptyFeatures := svc.entitlements(nilLic)
	if len(emptyFeatures) != 0 {
		t.Error("nil plan should return empty features")
	}

	names := svc.entitlementNames(lic)
	want := []string{"export", "max_users", "sla"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("entitlement names = %v, want %v", names, want)
	}
}
