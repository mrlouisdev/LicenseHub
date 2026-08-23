package handler

import "testing"

func TestOTPHashIsKeyedAndDomainSeparated(t *testing.T) {
	const code = "123456"
	one := hashOTPCode("secret-one-that-is-long-enough", code)
	two := hashOTPCode("secret-two-that-is-long-enough", code)
	if one == two {
		t.Fatal("OTP digest must depend on the server secret")
	}
	if one == hashToken(code) {
		t.Fatal("OTP digest must be domain-separated from token hashes")
	}
	if one != hashOTPCode("secret-one-that-is-long-enough", code) {
		t.Fatal("OTP digest must be deterministic for verification")
	}
}
