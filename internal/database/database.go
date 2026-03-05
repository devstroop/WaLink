package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps a SQLite connection with a mutex for safe concurrent access.
type DB struct {
	mu sync.Mutex
	db *sql.DB
}

// AccountRecord is the persistent account row.
type AccountRecord struct {
	ID          string
	PhoneNumber string
	AccountName string
	DataDir     string
	Status      string
	CreatedAt   string
	UpdatedAt   string
}

// Open creates or opens the SQLite database at path, running migrations.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	conn, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	d := &DB{db: conn}
	if err := d.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return d, nil
}

func (d *DB) Close() error { return d.db.Close() }

func (d *DB) migrate() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`
		CREATE TABLE IF NOT EXISTS account (
			id            TEXT PRIMARY KEY,
			phone_number  TEXT NOT NULL UNIQUE,
			account_name  TEXT NOT NULL DEFAULT '',
			data_dir      TEXT NOT NULL,
			status        TEXT NOT NULL DEFAULT 'sleeping',
			created_at    TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
		);
	`)
	return err
}

// CreateAccount inserts a new account row.
func (d *DB) CreateAccount(rec *AccountRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.db.Exec(`
		INSERT INTO account (id, phone_number, account_name, data_dir, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.PhoneNumber, rec.AccountName, rec.DataDir, rec.Status, now, now,
	)
	return err
}

// GetAccount retrieves a single account by ID.
func (d *DB) GetAccount(id string) (*AccountRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	row := d.db.QueryRow(
		`SELECT id, phone_number, account_name, data_dir, status, created_at, updated_at
		 FROM account WHERE id = ?`, id)
	return scanAccount(row)
}

// GetAccountByPhone looks up an account by phone number.
func (d *DB) GetAccountByPhone(phone string) (*AccountRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	row := d.db.QueryRow(
		`SELECT id, phone_number, account_name, data_dir, status, created_at, updated_at
		 FROM account WHERE phone_number = ?`, phone)
	return scanAccount(row)
}

// ListAccounts returns all accounts, optionally filtered by status.
func (d *DB) ListAccounts(statusFilter string) ([]*AccountRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `SELECT id, phone_number, account_name, data_dir, status, created_at, updated_at FROM account`
	var args []any
	if statusFilter != "" {
		query += ` WHERE status = ?`
		args = append(args, statusFilter)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*AccountRecord
	for rows.Next() {
		var r AccountRecord
		if err := rows.Scan(&r.ID, &r.PhoneNumber, &r.AccountName, &r.DataDir, &r.Status, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// UpdateStatus sets an account's status.
func (d *DB) UpdateStatus(id, status string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`UPDATE account SET status = ?, updated_at = datetime('now') WHERE id = ?`, status, id)
	return err
}

// UpdateAccountName sets an account's display name.
func (d *DB) UpdateAccountName(id, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`UPDATE account SET account_name = ?, updated_at = datetime('now') WHERE id = ?`, name, id)
	return err
}

// UpdatePhoneNumber sets an account's phone number.
func (d *DB) UpdatePhoneNumber(id, phone string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`UPDATE account SET phone_number = ?, updated_at = datetime('now') WHERE id = ?`, phone, id)
	return err
}

// DeleteAccount removes the account row.
func (d *DB) DeleteAccount(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`DELETE FROM account WHERE id = ?`, id)
	return err
}

func scanAccount(row *sql.Row) (*AccountRecord, error) {
	var r AccountRecord
	err := row.Scan(&r.ID, &r.PhoneNumber, &r.AccountName, &r.DataDir, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &r, err
}
