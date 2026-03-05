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
	CreatedAt   string
	UpdatedAt   string
}

// ProxyConfigRecord is the persistent proxy configuration row.
type ProxyConfigRecord struct {
	AccountID string
	Protocol  string // http, https, socks5
	Host      string
	Port      int
	Username  string
	Password  string
	Enabled   bool
}

// MessageRecord is a stored message row.
type MessageRecord struct {
	ID        string // whatsmeow message ID
	AccountID string
	ChatJID   string
	SenderJID string
	FromMe    bool
	Type      string // text, image, video, audio, document, sticker, reaction, other
	Body      string // text body, caption, or reaction emoji
	MediaType string // MIME type for media messages
	Timestamp string // RFC3339
}

// WebhookConfigRecord is the persistent webhook config row.
type WebhookConfigRecord struct {
	AccountID string
	URL       string
	Secret    string // optional HMAC signing secret
	Events    string // comma-separated event types, empty = all
	Enabled   bool
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
			created_at    TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
		);
	`)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(`
		CREATE TABLE IF NOT EXISTS proxy_config (
			account_id TEXT PRIMARY KEY REFERENCES account(id) ON DELETE CASCADE,
			protocol   TEXT NOT NULL DEFAULT 'http',
			host       TEXT NOT NULL,
			port       INTEGER NOT NULL,
			username   TEXT NOT NULL DEFAULT '',
			password   TEXT NOT NULL DEFAULT '',
			enabled    INTEGER NOT NULL DEFAULT 1
		);
	`)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(`
		CREATE TABLE IF NOT EXISTS message (
			id         TEXT NOT NULL,
			account_id TEXT NOT NULL REFERENCES account(id) ON DELETE CASCADE,
			chat_jid   TEXT NOT NULL,
			sender_jid TEXT NOT NULL,
			from_me    INTEGER NOT NULL DEFAULT 0,
			type       TEXT NOT NULL DEFAULT 'text',
			body       TEXT NOT NULL DEFAULT '',
			media_type TEXT NOT NULL DEFAULT '',
			timestamp  TEXT NOT NULL,
			PRIMARY KEY (account_id, id)
		);
	`)
	if err != nil {
		return err
	}

	// Index for paginated chat history queries
	_, err = d.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_message_chat_ts ON message (account_id, chat_jid, timestamp DESC);
	`)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(`
		CREATE TABLE IF NOT EXISTS webhook_config (
			account_id TEXT PRIMARY KEY REFERENCES account(id) ON DELETE CASCADE,
			url        TEXT NOT NULL,
			secret     TEXT NOT NULL DEFAULT '',
			events     TEXT NOT NULL DEFAULT '',
			enabled    INTEGER NOT NULL DEFAULT 1
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
		INSERT INTO account (id, phone_number, account_name, data_dir, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.PhoneNumber, rec.AccountName, rec.DataDir, now, now,
	)
	return err
}

// GetAccount retrieves a single account by ID.
func (d *DB) GetAccount(id string) (*AccountRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	row := d.db.QueryRow(
		`SELECT id, phone_number, account_name, data_dir, created_at, updated_at
		 FROM account WHERE id = ?`, id)
	return scanAccount(row)
}

// GetAccountByPhone looks up an account by phone number.
func (d *DB) GetAccountByPhone(phone string) (*AccountRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	row := d.db.QueryRow(
		`SELECT id, phone_number, account_name, data_dir, created_at, updated_at
		 FROM account WHERE phone_number = ?`, phone)
	return scanAccount(row)
}

// ListAccounts returns all accounts.
func (d *DB) ListAccounts() ([]*AccountRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `SELECT id, phone_number, account_name, data_dir, created_at, updated_at FROM account ORDER BY created_at DESC`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*AccountRecord
	for rows.Next() {
		var r AccountRecord
		if err := rows.Scan(&r.ID, &r.PhoneNumber, &r.AccountName, &r.DataDir, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
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
	err := row.Scan(&r.ID, &r.PhoneNumber, &r.AccountName, &r.DataDir, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &r, err
}

// ─── Proxy Config CRUD ──────────────────────────────────────

// UpsertProxyConfig inserts or replaces the proxy config for an account.
func (d *DB) UpsertProxyConfig(rec *ProxyConfigRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`
		INSERT INTO proxy_config (account_id, protocol, host, port, username, password, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET
			protocol = excluded.protocol,
			host     = excluded.host,
			port     = excluded.port,
			username = excluded.username,
			password = excluded.password,
			enabled  = excluded.enabled`,
		rec.AccountID, rec.Protocol, rec.Host, rec.Port, rec.Username, rec.Password, rec.Enabled,
	)
	return err
}

// GetProxyConfig returns the proxy config for an account, or nil if none.
func (d *DB) GetProxyConfig(accountID string) (*ProxyConfigRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	row := d.db.QueryRow(
		`SELECT account_id, protocol, host, port, username, password, enabled
		 FROM proxy_config WHERE account_id = ?`, accountID)
	var r ProxyConfigRecord
	var enabled int
	err := row.Scan(&r.AccountID, &r.Protocol, &r.Host, &r.Port, &r.Username, &r.Password, &enabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Enabled = enabled != 0
	return &r, nil
}

// DeleteProxyConfig removes the proxy config for an account.
func (d *DB) DeleteProxyConfig(accountID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`DELETE FROM proxy_config WHERE account_id = ?`, accountID)
	return err
}

// ─── Message CRUD ───────────────────────────────────────────

// InsertMessage stores a message row (idempotent — ignores duplicates).
func (d *DB) InsertMessage(rec *MessageRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`
		INSERT OR IGNORE INTO message (id, account_id, chat_jid, sender_jid, from_me, type, body, media_type, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.AccountID, rec.ChatJID, rec.SenderJID, rec.FromMe, rec.Type, rec.Body, rec.MediaType, rec.Timestamp,
	)
	return err
}

// ListMessages returns messages for a chat, ordered newest-first with cursor pagination.
// If before is non-empty, only messages with timestamp < before are returned.
func (d *DB) ListMessages(accountID, chatJID string, limit int, before string) ([]*MessageRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var (
		rows *sql.Rows
		err  error
	)
	if before != "" {
		rows, err = d.db.Query(`
			SELECT id, account_id, chat_jid, sender_jid, from_me, type, body, media_type, timestamp
			FROM message
			WHERE account_id = ? AND chat_jid = ? AND timestamp < ?
			ORDER BY timestamp DESC
			LIMIT ?`, accountID, chatJID, before, limit)
	} else {
		rows, err = d.db.Query(`
			SELECT id, account_id, chat_jid, sender_jid, from_me, type, body, media_type, timestamp
			FROM message
			WHERE account_id = ? AND chat_jid = ?
			ORDER BY timestamp DESC
			LIMIT ?`, accountID, chatJID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*MessageRecord
	for rows.Next() {
		var r MessageRecord
		if err := rows.Scan(&r.ID, &r.AccountID, &r.ChatJID, &r.SenderJID, &r.FromMe, &r.Type, &r.Body, &r.MediaType, &r.Timestamp); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// ─── Webhook Config CRUD ────────────────────────────────────

// UpsertWebhookConfig inserts or replaces the webhook config for an account.
func (d *DB) UpsertWebhookConfig(rec *WebhookConfigRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`
		INSERT INTO webhook_config (account_id, url, secret, events, enabled)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET
			url     = excluded.url,
			secret  = excluded.secret,
			events  = excluded.events,
			enabled = excluded.enabled`,
		rec.AccountID, rec.URL, rec.Secret, rec.Events, rec.Enabled,
	)
	return err
}

// GetWebhookConfig returns the webhook config for an account, or nil if none.
func (d *DB) GetWebhookConfig(accountID string) (*WebhookConfigRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	row := d.db.QueryRow(
		`SELECT account_id, url, secret, events, enabled
		 FROM webhook_config WHERE account_id = ?`, accountID)
	var r WebhookConfigRecord
	var enabled int
	err := row.Scan(&r.AccountID, &r.URL, &r.Secret, &r.Events, &enabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Enabled = enabled != 0
	return &r, nil
}

// DeleteWebhookConfig removes the webhook config for an account.
func (d *DB) DeleteWebhookConfig(accountID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`DELETE FROM webhook_config WHERE account_id = ?`, accountID)
	return err
}
