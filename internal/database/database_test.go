package database

import (
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenAndMigrate(t *testing.T) {
	db := openTestDB(t)
	// Verify table exists by listing (should return empty)
	records, err := db.ListAccounts("")
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestCreateAndGetAccount(t *testing.T) {
	db := openTestDB(t)

	rec := &AccountRecord{
		ID:          "test-id-1",
		PhoneNumber: "919876543210",
		AccountName: "test-account",
		DataDir:     "/tmp/test",
		Status:      "sleeping",
	}
	if err := db.CreateAccount(rec); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	got, err := db.GetAccount("test-id-1")
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if got == nil {
		t.Fatal("expected account, got nil")
	}
	if got.PhoneNumber != "919876543210" {
		t.Errorf("expected phone 919876543210, got %s", got.PhoneNumber)
	}
	if got.AccountName != "test-account" {
		t.Errorf("expected name test-account, got %s", got.AccountName)
	}
	if got.Status != "sleeping" {
		t.Errorf("expected status sleeping, got %s", got.Status)
	}
}

func TestGetAccountNotFound(t *testing.T) {
	db := openTestDB(t)

	got, err := db.GetAccount("nonexistent")
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent account, got %+v", got)
	}
}

func TestGetAccountByPhone(t *testing.T) {
	db := openTestDB(t)

	rec := &AccountRecord{
		ID: "id-phone-test", PhoneNumber: "1234567890",
		AccountName: "phone-test", DataDir: "/tmp/pt", Status: "sleeping",
	}
	db.CreateAccount(rec)

	got, err := db.GetAccountByPhone("1234567890")
	if err != nil {
		t.Fatalf("GetAccountByPhone: %v", err)
	}
	if got == nil || got.ID != "id-phone-test" {
		t.Errorf("expected id-phone-test, got %+v", got)
	}

	// Not found
	got, err = db.GetAccountByPhone("0000000000")
	if err != nil {
		t.Fatalf("GetAccountByPhone: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestDuplicatePhone(t *testing.T) {
	db := openTestDB(t)

	rec := &AccountRecord{
		ID: "dup-1", PhoneNumber: "5551234567",
		AccountName: "first", DataDir: "/tmp/d1", Status: "sleeping",
	}
	if err := db.CreateAccount(rec); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	rec2 := &AccountRecord{
		ID: "dup-2", PhoneNumber: "5551234567",
		AccountName: "second", DataDir: "/tmp/d2", Status: "sleeping",
	}
	err := db.CreateAccount(rec2)
	if err == nil {
		t.Error("expected UNIQUE constraint error, got nil")
	}
}

func TestListAccounts(t *testing.T) {
	db := openTestDB(t)

	for i, phone := range []string{"1111111111", "2222222222", "3333333333"} {
		db.CreateAccount(&AccountRecord{
			ID: phone, PhoneNumber: phone,
			AccountName: "acct", DataDir: "/tmp/" + phone,
			Status: func() string {
				if i == 2 {
					return "active"
				}
				return "sleeping"
			}(),
		})
	}

	// All
	all, err := db.ListAccounts("")
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3, got %d", len(all))
	}

	// Filtered
	sleeping, err := db.ListAccounts("sleeping")
	if err != nil {
		t.Fatalf("ListAccounts(sleeping): %v", err)
	}
	if len(sleeping) != 2 {
		t.Errorf("expected 2 sleeping, got %d", len(sleeping))
	}

	active, err := db.ListAccounts("active")
	if err != nil {
		t.Fatalf("ListAccounts(active): %v", err)
	}
	if len(active) != 1 {
		t.Errorf("expected 1 active, got %d", len(active))
	}
}

func TestUpdateStatus(t *testing.T) {
	db := openTestDB(t)

	db.CreateAccount(&AccountRecord{
		ID: "status-test", PhoneNumber: "9999999999",
		AccountName: "st", DataDir: "/tmp/st", Status: "sleeping",
	})

	if err := db.UpdateStatus("status-test", "active"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	got, _ := db.GetAccount("status-test")
	if got.Status != "active" {
		t.Errorf("expected active, got %s", got.Status)
	}
}

func TestUpdateAccountName(t *testing.T) {
	db := openTestDB(t)

	db.CreateAccount(&AccountRecord{
		ID: "name-test", PhoneNumber: "8888888888",
		AccountName: "old-name", DataDir: "/tmp/nt", Status: "sleeping",
	})

	if err := db.UpdateAccountName("name-test", "new-name"); err != nil {
		t.Fatalf("UpdateAccountName: %v", err)
	}

	got, _ := db.GetAccount("name-test")
	if got.AccountName != "new-name" {
		t.Errorf("expected new-name, got %s", got.AccountName)
	}
}

func TestDeleteAccount(t *testing.T) {
	db := openTestDB(t)

	db.CreateAccount(&AccountRecord{
		ID: "del-test", PhoneNumber: "7777777777",
		AccountName: "del", DataDir: "/tmp/del", Status: "sleeping",
	})

	if err := db.DeleteAccount("del-test"); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}

	got, _ := db.GetAccount("del-test")
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}
