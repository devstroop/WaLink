package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/devstroop/walink/internal/config"
	"github.com/devstroop/walink/internal/database"
	"github.com/devstroop/walink/internal/model"
)

// AccountManager manages the lifecycle of all WhatsApp accounts.
type AccountManager struct {
	mu       sync.RWMutex
	accounts map[string]*Account // keyed by account ID

	cfg     *config.Config
	db      *database.DB
	baseDir string
}

// NewAccountManager creates a new manager.
func NewAccountManager(cfg *config.Config, db *database.DB) (*AccountManager, error) {
	baseDir := cfg.Accounts.BaseDirectory
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("create base dir: %w", err)
	}
	return &AccountManager{
		accounts: make(map[string]*Account),
		cfg:      cfg,
		db:       db,
		baseDir:  baseDir,
	}, nil
}

// CreateAccount validates input, persists to DB, and returns the new account.
func (m *AccountManager) CreateAccount(req model.CreateAccountRequest) (*model.CreateAccountResponse, error) {
	phone := NormalizePhone(req.PhoneNumber)
	if len(phone) < 7 || len(phone) > 15 {
		return nil, fmt.Errorf("invalid phone number: must be 7-15 digits")
	}

	// Uniqueness check
	existing, err := m.db.GetAccountByPhone(phone)
	if err != nil {
		return nil, fmt.Errorf("db lookup: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("phone number '%s' already exists", phone)
	}

	id := NewUUID()
	dataDir := filepath.Join(m.baseDir, id)

	name := req.AccountName
	if name == "" {
		name = "unknown"
	}

	now := time.Now().UTC()
	rec := &database.AccountRecord{
		ID:          id,
		PhoneNumber: phone,
		AccountName: name,
		DataDir:     dataDir,
		Status:      string(model.StatusSleeping),
	}
	if err := m.db.CreateAccount(rec); err != nil {
		return nil, fmt.Errorf("db insert: %w", err)
	}

	acct := NewAccount(id, phone, name, dataDir, now)

	m.mu.Lock()
	m.accounts[id] = acct
	m.mu.Unlock()

	return &model.CreateAccountResponse{
		ID:          id,
		PhoneNumber: phone,
		AccountName: name,
		Status:      "created",
		CreatedAt:   now.Format(time.RFC3339),
	}, nil
}

// GetAccount returns the in-memory account, if loaded.
func (m *AccountManager) GetAccount(id string) *Account {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.accounts[id]
}

// ListAccounts returns info for all known accounts.
func (m *AccountManager) ListAccounts() model.AccountListResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []model.AccountInfo
	for _, acct := range m.accounts {
		list = append(list, acct.Info())
	}
	return model.AccountListResponse{Accounts: list, Total: len(list)}
}

// DeleteAccount removes an account from memory, DB, and optionally disk.
func (m *AccountManager) DeleteAccount(id string, deleteData bool) (*model.DeleteAccountResponse, error) {
	m.mu.Lock()
	acct, ok := m.accounts[id]
	if ok {
		delete(m.accounts, id)
	}
	m.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("account not found")
	}

	acct.Disconnect()

	if err := m.db.DeleteAccount(id); err != nil {
		return nil, fmt.Errorf("db delete: %w", err)
	}

	if deleteData {
		_ = os.RemoveAll(acct.DataDir)
	}

	return &model.DeleteAccountResponse{
		Message:     "account deleted",
		AccountID:   id,
		DataDeleted: deleteData,
	}, nil
}

// ConnectAccount ensures the account's whatsmeow client is connected.
func (m *AccountManager) ConnectAccount(ctx context.Context, id string) error {
	acct := m.GetAccount(id)
	if acct == nil {
		return fmt.Errorf("account not found")
	}
	return acct.EnsureConnected(ctx)
}

// UpdateAccountName updates the display name in memory and DB.
func (m *AccountManager) UpdateAccountName(id, name string) error {
	acct := m.GetAccount(id)
	if acct == nil {
		return fmt.Errorf("account not found")
	}
	if err := m.db.UpdateAccountName(id, name); err != nil {
		return fmt.Errorf("db update name: %w", err)
	}
	acct.AccountName = name
	return nil
}

// UpdatePhoneNumber updates the phone number in memory and DB.
func (m *AccountManager) UpdatePhoneNumber(id, phone string) error {
	phone = NormalizePhone(phone)
	if len(phone) < 7 || len(phone) > 15 {
		return fmt.Errorf("invalid phone number: must be 7-15 digits")
	}
	acct := m.GetAccount(id)
	if acct == nil {
		return fmt.Errorf("account not found")
	}
	existing, err := m.db.GetAccountByPhone(phone)
	if err != nil {
		return fmt.Errorf("db lookup: %w", err)
	}
	if existing != nil && existing.ID != id {
		return fmt.Errorf("phone number '%s' already exists", phone)
	}
	if err := m.db.UpdatePhoneNumber(id, phone); err != nil {
		return fmt.Errorf("db update phone: %w", err)
	}
	acct.PhoneNumber = phone
	return nil
}

// DiscoverAccounts loads all DB accounts into memory (called at startup).
func (m *AccountManager) DiscoverAccounts(ctx context.Context) error {
	records, err := m.db.ListAccounts("")
	if err != nil {
		return fmt.Errorf("list accounts: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, rec := range records {
		if _, ok := m.accounts[rec.ID]; ok {
			continue
		}

		created, _ := time.Parse(time.RFC3339, rec.CreatedAt)
		acct := NewAccount(rec.ID, rec.PhoneNumber, rec.AccountName, rec.DataDir, created)
		m.accounts[rec.ID] = acct
		log.Info().Str("id", rec.ID).Str("phone", rec.PhoneNumber).Msg("discovered account")
	}

	log.Info().Int("count", len(m.accounts)).Msg("accounts loaded")
	return nil
}

// ShutdownAll disconnects every active account.
func (m *AccountManager) ShutdownAll() {
	m.mu.RLock()
	ids := make([]string, 0, len(m.accounts))
	for id := range m.accounts {
		ids = append(ids, id)
	}
	m.mu.RUnlock()

	for _, id := range ids {
		if acct := m.GetAccount(id); acct != nil {
			acct.Disconnect()
		}
	}
	log.Info().Int("count", len(ids)).Msg("all accounts disconnected")
}
