package service

import (
	"testing"
	"time"
)

func TestNormalizePhone(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"+919876543210", "919876543210"},
		{"91 9876 543 210", "919876543210"},
		{"1234567890", "1234567890"},
		{"+1 (555) 123-4567", "15551234567"},
		{"", ""},
	}
	for _, tc := range cases {
		got := NormalizePhone(tc.input)
		if got != tc.want {
			t.Errorf("NormalizePhone(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestPhoneToJID(t *testing.T) {
	got := PhoneToJID("+919876543210")
	want := "919876543210@s.whatsapp.net"
	if got != want {
		t.Errorf("PhoneToJID = %q, want %q", got, want)
	}
}

func TestNewUUID(t *testing.T) {
	id1 := NewUUID()
	id2 := NewUUID()
	if id1 == "" {
		t.Error("NewUUID returned empty string")
	}
	if id1 == id2 {
		t.Error("NewUUID returned duplicate")
	}
	if len(id1) != 36 {
		t.Errorf("expected UUID length 36, got %d", len(id1))
	}
}

func TestNewAccount(t *testing.T) {
	now := time.Now()
	acct := NewAccount("test-id", "919876543210", "main", "/tmp/test", now)

	if acct.ID != "test-id" {
		t.Errorf("expected ID test-id, got %s", acct.ID)
	}
	if acct.PhoneNumber != "919876543210" {
		t.Errorf("expected phone 919876543210, got %s", acct.PhoneNumber)
	}
	if acct.AccountName != "main" {
		t.Errorf("expected name main, got %s", acct.AccountName)
	}
	if acct.DataDir != "/tmp/test" {
		t.Errorf("expected data dir /tmp/test, got %s", acct.DataDir)
	}
}

func TestAccountInfo(t *testing.T) {
	now := time.Now()
	acct := NewAccount("info-id", "1234567890", "info-acct", "/tmp/info", now)

	info := acct.Info()
	if info.ID != "info-id" {
		t.Errorf("expected ID info-id, got %s", info.ID)
	}
	if info.AccountName != "info-acct" {
		t.Errorf("expected name info-acct, got %s", info.AccountName)
	}
	if info.Authorized {
		t.Error("expected Authorized=false for new account")
	}
	if info.PhoneNumber == nil || *info.PhoneNumber != "1234567890" {
		t.Errorf("expected phone 1234567890, got %v", info.PhoneNumber)
	}
}

func TestAccountInfoNoPhone(t *testing.T) {
	acct := NewAccount("no-phone", "", "nophone", "/tmp/np", time.Now())
	info := acct.Info()
	if info.PhoneNumber != nil {
		t.Errorf("expected nil phone, got %v", info.PhoneNumber)
	}
}

func TestAccountStatusResponse(t *testing.T) {
	acct := NewAccount("sr-id", "5551234567", "sr", "/tmp/sr", time.Now())
	resp := acct.StatusResponse()

	if resp.AccountID != "sr-id" {
		t.Errorf("expected account_id sr-id, got %s", resp.AccountID)
	}
	if resp.Authorized {
		t.Error("expected Authorized=false")
	}
	if resp.PhoneNumber == nil || *resp.PhoneNumber != "5551234567" {
		t.Errorf("expected phone 5551234567, got %v", resp.PhoneNumber)
	}
}

func TestAccountDisconnectWhileAlreadySleeping(t *testing.T) {
	acct := NewAccount("disc-id", "2222222222", "disc", "/tmp/disc", time.Now())
	// Should not panic when client is nil
	acct.Disconnect()
	// Client should still be nil after disconnect
	if acct.IsLoggedIn() {
		t.Error("expected IsLoggedIn=false after disconnect")
	}
}
