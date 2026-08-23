package service

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestOTPLoggingNeverContainsRecipientOrCode(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	mail := NewEmailService("", "587", "", "", "", logger, nil)
	const recipient = "otp-recipient@example.test"
	const code = "381927"
	if err := mail.SendOTPCode(recipient, code); err != nil {
		t.Fatalf("disabled SMTP should be a no-op at the service boundary: %v", err)
	}
	output := logs.String()
	if strings.Contains(output, recipient) || strings.Contains(output, code) {
		t.Fatalf("sensitive OTP metadata reached logs: %s", output)
	}
}
