package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/itsalfredakku/walink/internal/config"
	"github.com/itsalfredakku/walink/internal/database"
	"github.com/itsalfredakku/walink/internal/model"
)

func setupManager(t *testing.T) *AccountManager {
	t.Helper()
	dir := t.TempDir()

	dbPath := filepath.Join(dir, "db", "test.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("Open DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cfg := &config.Config{
		Accounts: config.AccountsConfig{
			BaseDirectory: filepath.Join(dir, "accounts"),
			Defaults:      config.AccountDefaultsConfig{IdleTimeout: 300},
		},
	}

	mgr, err := NewAccountManager(cfg, db)
	if err != nil {
		t.Fatalf("NewAccountManager: %v", err)
	}
	return mgr
}

func TestManagerCreateAccount(t *testing.T) {
	mgr := setupManager(t)

	resp, err := mgr.CreateAccount(model.CreateAccountRequest{
		PhoneNumber: "+919876543210",
		AccountName: "main",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if resp.PhoneNumber != "919876543210" {
		t.Errorf("expected normalized phone 919876543210, got %s", resp.PhoneNumber)
	}
	if resp.AccountName != "main" {
		t.Errorf("expected name main, got %s", resp.AccountName)
	}
	if resp.Status != "created" {
		t.Errorf("expected status created, got %s", resp.Status)
	}
	if resp.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestManagerCreateAccountDefaultName(t *testing.T) {
	mgr := setupManager(t)

	resp, err := mgr.CreateAccount(model.CreateAccountRequest{
		PhoneNumber: "1234567890",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if resp.AccountName != "unknown" {
		t.Errorf("expected default name 'unknown', got %s", resp.AccountName)
	}
}

func TestManagerCreateAccountInvalidPhone(t *testing.T) {
	mgr := setupManager(t)

	_, err := mgr.CreateAccount(model.CreateAccountRequest{
		PhoneNumber: "123", // too short
	})
	if err == nil {
		t.Error("expected error for short phone number")
	}
}

func TestManagerCreateAccountDuplicatePhone(t *testing.T) {
	mgr := setupManager(t)

	mgr.CreateAccount(model.CreateAccountRequest{
		PhoneNumber: "9876543210",
		AccountName: "first",
	})

	_, err := mgr.CreateAccount(model.CreateAccountRequest{
		PhoneNumber: "9876543210",
		AccountName: "second",
	})
	if err == nil {
		t.Error("expected error for duplicate phone")
	}
}

func TestManagerGetAccount(t *testing.T) {
	mgr := setupManager(t)

	resp, _ := mgr.CreateAccount(model.CreateAccountRequest{
		PhoneNumber: "5551234567",
		AccountName: "find-me",
	})

	acct := mgr.GetAccount(resp.ID)
	if acct == nil {
		t.Fatal("expected to find account, got nil")
	}
	if acct.AccountName != "find-me" {
		t.Errorf("expected name find-me, got %s", acct.AccountName)
	}
}

func TestManagerGetAccountNotFound(t *testing.T) {
	mgr := setupManager(t)

	acct := mgr.GetAccount("nonexistent-id")
	if acct != nil {
		t.Errorf("expected nil, got %+v", acct)
	}
}

func TestManagerListAccounts(t *testing.T) {
	mgr := setupManager(t)

	mgr.CreateAccount(model.CreateAccountRequest{PhoneNumber: "1111111111", AccountName: "a1"})
	mgr.CreateAccount(model.CreateAccountRequest{PhoneNumber: "2222222222", AccountName: "a2"})

	list := mgr.ListAccounts()
	if list.Total != 2 {
		t.Errorf("expected 2 accounts, got %d", list.Total)
	}
}

func TestManagerDeleteAccount(t *testing.T) {
	mgr := setupManager(t)

	resp, _ := mgr.CreateAccount(model.CreateAccountRequest{
		PhoneNumber: "3333333333",
		AccountName: "deletable",
	})

	delResp, err := mgr.DeleteAccount(resp.ID, false)
	if err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if delResp.AccountID != resp.ID {
		t.Errorf("expected account_id %s, got %s", resp.ID, delResp.AccountID)
	}
	if delResp.DataDeleted {
		t.Error("expected DataDeleted=false")
	}

	// Should be gone
	acct := mgr.GetAccount(resp.ID)
	if acct != nil {
		t.Error("expected nil after delete")
	}
}

func TestManagerDeleteAccountNotFound(t *testing.T) {
	mgr := setupManager(t)

	_, err := mgr.DeleteAccount("nonexistent", false)
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

func TestManagerDiscoverAccounts(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db", "test.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("Open DB: %v", err)
	}
	defer db.Close()

	// Pre-seed the DB
	db.CreateAccount(&database.AccountRecord{
		ID: "pre-existing", PhoneNumber: "4444444444",
		AccountName: "pre", DataDir: filepath.Join(dir, "accounts", "pre-existing"),
		IdleTimeout: 300, Status: "sleeping",
	})

	cfg := &config.Config{
		Accounts: config.AccountsConfig{
			BaseDirectory: filepath.Join(dir, "accounts"),
			Defaults:      config.AccountDefaultsConfig{IdleTimeout: 300},
		},
	}

	mgr, _ := NewAccountManager(cfg, db)

	if err := mgr.DiscoverAccounts(context.Background()); err != nil {
		t.Fatalf("DiscoverAccounts: %v", err)
	}

	acct := mgr.GetAccount("pre-existing")
	if acct == nil {
		t.Fatal("expected to discover pre-existing account")
	}
	if acct.PhoneNumber != "4444444444" {
		t.Errorf("expected phone 4444444444, got %s", acct.PhoneNumber)
	}
}

func TestManagerCreateAccountCustomIdleTimeout(t *testing.T) {
	mgr := setupManager(t)

	timeout := int64(600)
	resp, err := mgr.CreateAccount(model.CreateAccountRequest{
		PhoneNumber: "7777777777",
		AccountName: "custom-timeout",
		IdleTimeout: &timeout,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	acct := mgr.GetAccount(resp.ID)
	if acct.IdleTimeout != 600 {
		t.Errorf("expected idle_timeout 600, got %d", acct.IdleTimeout)
	}
}
