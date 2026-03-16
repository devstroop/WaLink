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
	UserID      string // FK to user.id (empty = unassigned / legacy)
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
	ID        string // message ID
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

// RoleRecord is a persistent role row.
type RoleRecord struct {
	ID          string
	Name        string
	Description string
	IsBuiltin   bool // built-in roles (admin, user) cannot be deleted
	CreatedAt   string
}

// UserRecord is a persistent user row.
type UserRecord struct {
	ID           string
	Username     string
	Email        string
	PasswordHash string
	RoleID       string
	RoleName     string // joined from role table (read-only)
	Enabled      bool
	CreatedAt    string
	UpdatedAt    string
}

// APIKeyRecord is a persistent API key row.
type APIKeyRecord struct {
	ID        string
	UserID    string  // FK to user.id
	AccountID *string // FK to account.id, nil = not scoped to a specific account
	Name      string  // human-readable label
	Prefix    string  // first 8 chars of key for identification
	KeyHash   string  // SHA-256 hash of full key
	ExpiresAt *string // RFC3339, nil = never expires
	LastUsed  *string // RFC3339, nil = never used
	Enabled   bool
	CreatedAt string
}

// ResetTokenRecord is a password-reset token stored in the DB.
type ResetTokenRecord struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt string // RFC3339
	Used      bool
	CreatedAt string
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
	if err != nil {
		return err
	}

	// ── RBAC tables ─────────────────────────────────

	_, err = d.db.Exec(`
		CREATE TABLE IF NOT EXISTS role (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			is_builtin  INTEGER NOT NULL DEFAULT 0,
			created_at  TEXT NOT NULL DEFAULT (datetime('now'))
		);
	`)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(`
		CREATE TABLE IF NOT EXISTS role_permission (
			role_id    TEXT NOT NULL REFERENCES role(id) ON DELETE CASCADE,
			permission TEXT NOT NULL,
			PRIMARY KEY (role_id, permission)
		);
	`)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(`
		CREATE TABLE IF NOT EXISTS user (
			id            TEXT PRIMARY KEY,
			username      TEXT NOT NULL UNIQUE,
			email         TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL,
			role_id       TEXT NOT NULL REFERENCES role(id),
			enabled       INTEGER NOT NULL DEFAULT 1,
			created_at    TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
		);
	`)
	if err != nil {
		return err
	}

	// Migration: add email column to user if missing (upgrade from earlier schema)
	d.db.Exec(`ALTER TABLE user ADD COLUMN email TEXT NOT NULL DEFAULT ''`)
	if err != nil {
		return err
	}

	// Add user_id to account (idempotent — SQLite ignores if column exists)
	// NULL means no user assigned (legacy/unassigned accounts). FK only enforced when non-NULL.
	d.db.Exec(`ALTER TABLE account ADD COLUMN user_id TEXT REFERENCES user(id) DEFAULT NULL`)

	// ── API key table ───────────────────────────────
	_, err = d.db.Exec(`
		CREATE TABLE IF NOT EXISTS api_key (
			id         TEXT PRIMARY KEY,
			user_id    TEXT NOT NULL REFERENCES user(id) ON DELETE CASCADE,
			account_id TEXT REFERENCES account(id) ON DELETE CASCADE,
			name       TEXT NOT NULL DEFAULT '',
			prefix     TEXT NOT NULL DEFAULT '',
			key_hash   TEXT NOT NULL UNIQUE,
			expires_at TEXT,
			last_used  TEXT,
			enabled    INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
	`)
	if err != nil {
		return err
	}

	// Migration: add account_id column to api_key if missing (upgrade from earlier schema)
	d.db.Exec(`ALTER TABLE api_key ADD COLUMN account_id TEXT REFERENCES account(id) ON DELETE CASCADE`)

	// ── Password reset token table ──────────────────
	_, err = d.db.Exec(`
		CREATE TABLE IF NOT EXISTS password_reset_token (
			id         TEXT PRIMARY KEY,
			user_id    TEXT NOT NULL REFERENCES user(id) ON DELETE CASCADE,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TEXT NOT NULL,
			used       INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
	`)
	if err != nil {
		return err
	}

	// ── Seed built-in roles ─────────────────────────
	if err := d.seedRoles(); err != nil {
		return err
	}

	return nil
}

// seedRoles inserts the built-in admin and user roles with default permissions.
func (d *DB) seedRoles() error {
	// Admin role — wildcard permission
	_, err := d.db.Exec(`INSERT OR IGNORE INTO role (id, name, description, is_builtin) VALUES ('builtin-admin', 'admin', 'Full system access', 1)`)
	if err != nil {
		return err
	}
	_, err = d.db.Exec(`INSERT OR IGNORE INTO role_permission (role_id, permission) VALUES ('builtin-admin', '*')`)
	if err != nil {
		return err
	}

	// User role — restricted permissions
	_, err = d.db.Exec(`INSERT OR IGNORE INTO role (id, name, description, is_builtin) VALUES ('builtin-user', 'user', 'Standard user access', 1)`)
	if err != nil {
		return err
	}

	userPerms := []string{
		"accounts:read",
		"session:*",
		"messages:*",
		"chats:read",
		"contacts:*",
		"groups:*",
		"presence:*",
		"profile:*",
		"api-keys:*",
	}
	for _, p := range userPerms {
		_, err = d.db.Exec(`INSERT OR IGNORE INTO role_permission (role_id, permission) VALUES ('builtin-user', ?)`, p)
		if err != nil {
			return err
		}
	}

	return nil
}

// CreateAccount inserts a new account row.
func (d *DB) CreateAccount(rec *AccountRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	// Map empty UserID to NULL for FK compatibility
	var userID any
	if rec.UserID != "" {
		userID = rec.UserID
	}
	_, err := d.db.Exec(`
		INSERT INTO account (id, phone_number, account_name, data_dir, user_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.PhoneNumber, rec.AccountName, rec.DataDir, userID, now, now,
	)
	return err
}

// GetAccount retrieves a single account by ID.
func (d *DB) GetAccount(id string) (*AccountRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	row := d.db.QueryRow(
		`SELECT id, phone_number, account_name, data_dir, COALESCE(user_id, ''), created_at, updated_at
		 FROM account WHERE id = ?`, id)
	return scanAccount(row)
}

// GetAccountByPhone looks up an account by phone number.
func (d *DB) GetAccountByPhone(phone string) (*AccountRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	row := d.db.QueryRow(
		`SELECT id, phone_number, account_name, data_dir, COALESCE(user_id, ''), created_at, updated_at
		 FROM account WHERE phone_number = ?`, phone)
	return scanAccount(row)
}

// ListAccounts returns all accounts.
func (d *DB) ListAccounts() ([]*AccountRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `SELECT id, phone_number, account_name, data_dir, COALESCE(user_id, ''), created_at, updated_at FROM account ORDER BY created_at DESC`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*AccountRecord
	for rows.Next() {
		var r AccountRecord
		if err := rows.Scan(&r.ID, &r.PhoneNumber, &r.AccountName, &r.DataDir, &r.UserID, &r.CreatedAt, &r.UpdatedAt); err != nil {
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

// UpdateAccountUserID assigns an account to a user.
func (d *DB) UpdateAccountUserID(id, userID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	var uid any
	if userID != "" {
		uid = userID
	}
	_, err := d.db.Exec(`UPDATE account SET user_id = ?, updated_at = datetime('now') WHERE id = ?`, uid, id)
	return err
}

// ListAccountsByUser returns accounts belonging to a specific user.
func (d *DB) ListAccountsByUser(userID string) ([]*AccountRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(
		`SELECT id, phone_number, account_name, data_dir, COALESCE(user_id, ''), created_at, updated_at
		 FROM account WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*AccountRecord
	for rows.Next() {
		var r AccountRecord
		if err := rows.Scan(&r.ID, &r.PhoneNumber, &r.AccountName, &r.DataDir, &r.UserID, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

func scanAccount(row *sql.Row) (*AccountRecord, error) {
	var r AccountRecord
	err := row.Scan(&r.ID, &r.PhoneNumber, &r.AccountName, &r.DataDir, &r.UserID, &r.CreatedAt, &r.UpdatedAt)
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

// LastMessageInfo holds the latest message summary for a single chat.
type LastMessageInfo struct {
	ChatJID   string
	Body      string
	SenderJID string
	FromMe    bool
	Timestamp string // RFC3339
}

// GetLastMessagePerChat returns the most recent message for each chat belonging to accountID.
func (d *DB) GetLastMessagePerChat(accountID string) (map[string]*LastMessageInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`
		SELECT m.chat_jid, m.body, m.sender_jid, m.from_me, m.timestamp
		FROM message m
		INNER JOIN (
			SELECT chat_jid, MAX(timestamp) AS max_ts
			FROM message
			WHERE account_id = ?
			GROUP BY chat_jid
		) latest ON m.chat_jid = latest.chat_jid AND m.timestamp = latest.max_ts AND m.account_id = ?
	`, accountID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*LastMessageInfo)
	for rows.Next() {
		var lm LastMessageInfo
		if err := rows.Scan(&lm.ChatJID, &lm.Body, &lm.SenderJID, &lm.FromMe, &lm.Timestamp); err != nil {
			return nil, err
		}
		result[lm.ChatJID] = &lm
	}
	return result, rows.Err()
}

// GetUnreadCountPerChat returns the number of unread (not from_me, not yet read-receipted) messages per chat.
// For now we approximate "unread" as messages from others received after the latest outgoing or read-marked message.
func (d *DB) GetUnreadCountPerChat(accountID string) (map[string]int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`
		SELECT chat_jid, COUNT(*) AS cnt
		FROM message
		WHERE account_id = ? AND from_me = 0
		  AND timestamp > COALESCE(
		    (SELECT MAX(m2.timestamp) FROM message m2
		     WHERE m2.account_id = message.account_id
		       AND m2.chat_jid  = message.chat_jid
		       AND m2.from_me = 1), '')
		GROUP BY chat_jid
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var jid string
		var cnt int
		if err := rows.Scan(&jid, &cnt); err != nil {
			return nil, err
		}
		result[jid] = cnt
	}
	return result, rows.Err()
}

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

// ─── Role CRUD ──────────────────────────────────────────────

// CreateRole inserts a new role.
func (d *DB) CreateRole(rec *RoleRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`INSERT INTO role (id, name, description, is_builtin) VALUES (?, ?, ?, ?)`,
		rec.ID, rec.Name, rec.Description, rec.IsBuiltin)
	return err
}

// GetRole retrieves a role by ID.
func (d *DB) GetRole(id string) (*RoleRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	row := d.db.QueryRow(`SELECT id, name, description, is_builtin, created_at FROM role WHERE id = ?`, id)
	var r RoleRecord
	var builtin int
	err := row.Scan(&r.ID, &r.Name, &r.Description, &builtin, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.IsBuiltin = builtin != 0
	return &r, nil
}

// GetRoleByName retrieves a role by name.
func (d *DB) GetRoleByName(name string) (*RoleRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	row := d.db.QueryRow(`SELECT id, name, description, is_builtin, created_at FROM role WHERE name = ?`, name)
	var r RoleRecord
	var builtin int
	err := row.Scan(&r.ID, &r.Name, &r.Description, &builtin, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.IsBuiltin = builtin != 0
	return &r, nil
}

// ListRoles returns all roles.
func (d *DB) ListRoles() ([]*RoleRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`SELECT id, name, description, is_builtin, created_at FROM role ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*RoleRecord
	for rows.Next() {
		var r RoleRecord
		var builtin int
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &builtin, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.IsBuiltin = builtin != 0
		out = append(out, &r)
	}
	return out, rows.Err()
}

// UpdateRole updates a role's name and description.
func (d *DB) UpdateRole(id, name, description string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`UPDATE role SET name = ?, description = ? WHERE id = ?`, name, description, id)
	return err
}

// DeleteRole removes a role. Built-in roles should be checked by the caller.
func (d *DB) DeleteRole(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`DELETE FROM role WHERE id = ?`, id)
	return err
}

// GetRolePermissions returns the permission strings for a role.
func (d *DB) GetRolePermissions(roleID string) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`SELECT permission FROM role_permission WHERE role_id = ? ORDER BY permission`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

// SetRolePermissions replaces all permissions for a role.
func (d *DB) SetRolePermissions(roleID string, permissions []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM role_permission WHERE role_id = ?`, roleID); err != nil {
		return err
	}
	for _, p := range permissions {
		if _, err := tx.Exec(`INSERT INTO role_permission (role_id, permission) VALUES (?, ?)`, roleID, p); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ─── User CRUD ──────────────────────────────────────────────

// CreateUser inserts a new user.
func (d *DB) CreateUser(rec *UserRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.db.Exec(`
		INSERT INTO user (id, username, email, password_hash, role_id, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.Username, rec.Email, rec.PasswordHash, rec.RoleID, rec.Enabled, now, now)
	return err
}

// GetUser retrieves a user by ID (joins role name).
func (d *DB) GetUser(id string) (*UserRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	row := d.db.QueryRow(`
		SELECT u.id, u.username, u.email, u.password_hash, u.role_id, r.name, u.enabled, u.created_at, u.updated_at
		FROM user u JOIN role r ON u.role_id = r.id
		WHERE u.id = ?`, id)
	return scanUser(row)
}

// GetUserByUsername retrieves a user by username.
func (d *DB) GetUserByUsername(username string) (*UserRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	row := d.db.QueryRow(`
		SELECT u.id, u.username, u.email, u.password_hash, u.role_id, r.name, u.enabled, u.created_at, u.updated_at
		FROM user u JOIN role r ON u.role_id = r.id
		WHERE u.username = ?`, username)
	return scanUser(row)
}

// GetUserByEmail retrieves a user by email address.
func (d *DB) GetUserByEmail(email string) (*UserRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	row := d.db.QueryRow(`
		SELECT u.id, u.username, u.email, u.password_hash, u.role_id, r.name, u.enabled, u.created_at, u.updated_at
		FROM user u JOIN role r ON u.role_id = r.id
		WHERE u.email = ?`, email)
	return scanUser(row)
}

// ListUsers returns all users.
func (d *DB) ListUsers() ([]*UserRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`
		SELECT u.id, u.username, u.email, u.password_hash, u.role_id, r.name, u.enabled, u.created_at, u.updated_at
		FROM user u JOIN role r ON u.role_id = r.id
		ORDER BY u.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*UserRecord
	for rows.Next() {
		var r UserRecord
		var enabled int
		if err := rows.Scan(&r.ID, &r.Username, &r.Email, &r.PasswordHash, &r.RoleID, &r.RoleName, &enabled, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Enabled = enabled != 0
		out = append(out, &r)
	}
	return out, rows.Err()
}

// UpdateUser updates a user's role and enabled status.
func (d *DB) UpdateUser(id, roleID string, enabled bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`UPDATE user SET role_id = ?, enabled = ?, updated_at = datetime('now') WHERE id = ?`,
		roleID, enabled, id)
	return err
}

// UpdateUserPassword updates a user's password hash.
func (d *DB) UpdateUserPassword(id, passwordHash string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`UPDATE user SET password_hash = ?, updated_at = datetime('now') WHERE id = ?`,
		passwordHash, id)
	return err
}

// UpdateUserEmail updates a user's email address.
func (d *DB) UpdateUserEmail(id, email string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`UPDATE user SET email = ?, updated_at = datetime('now') WHERE id = ?`,
		email, id)
	return err
}

// DeleteUser removes a user.
func (d *DB) DeleteUser(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`DELETE FROM user WHERE id = ?`, id)
	return err
}

// CountUsersByRole returns how many users have a given role.
func (d *DB) CountUsersByRole(roleID string) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var count int
	err := d.db.QueryRow(`SELECT COUNT(*) FROM user WHERE role_id = ?`, roleID).Scan(&count)
	return count, err
}

func scanUser(row *sql.Row) (*UserRecord, error) {
	var r UserRecord
	var enabled int
	err := row.Scan(&r.ID, &r.Username, &r.Email, &r.PasswordHash, &r.RoleID, &r.RoleName, &enabled, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Enabled = enabled != 0
	return &r, nil
}

// ─── API Key CRUD ───────────────────────────────────────────

// CreateAPIKey inserts a new API key.
func (d *DB) CreateAPIKey(rec *APIKeyRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.db.Exec(`
		INSERT INTO api_key (id, user_id, account_id, name, prefix, key_hash, expires_at, last_used, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)`,
		rec.ID, rec.UserID, rec.AccountID, rec.Name, rec.Prefix, rec.KeyHash, rec.ExpiresAt, rec.Enabled, now)
	return err
}

// GetAPIKey retrieves an API key by ID.
func (d *DB) GetAPIKey(id string) (*APIKeyRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	row := d.db.QueryRow(`SELECT id, user_id, account_id, name, prefix, key_hash, expires_at, last_used, enabled, created_at FROM api_key WHERE id = ?`, id)
	return scanAPIKey(row)
}

// GetAPIKeyByHash retrieves an API key by its SHA-256 hash.
func (d *DB) GetAPIKeyByHash(keyHash string) (*APIKeyRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	row := d.db.QueryRow(`SELECT id, user_id, account_id, name, prefix, key_hash, expires_at, last_used, enabled, created_at FROM api_key WHERE key_hash = ?`, keyHash)
	return scanAPIKey(row)
}

// ListAPIKeysByUser returns all API keys for a user.
func (d *DB) ListAPIKeysByUser(userID string) ([]*APIKeyRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`SELECT id, user_id, account_id, name, prefix, key_hash, expires_at, last_used, enabled, created_at FROM api_key WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*APIKeyRecord
	for rows.Next() {
		r, err := scanAPIKeyRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListAllAPIKeys returns all API keys (admin use).
func (d *DB) ListAllAPIKeys() ([]*APIKeyRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`SELECT id, user_id, account_id, name, prefix, key_hash, expires_at, last_used, enabled, created_at FROM api_key ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*APIKeyRecord
	for rows.Next() {
		r, err := scanAPIKeyRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteAPIKey removes an API key.
func (d *DB) DeleteAPIKey(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`DELETE FROM api_key WHERE id = ?`, id)
	return err
}

// UpdateAPIKeyLastUsed updates the last_used timestamp.
func (d *DB) UpdateAPIKeyLastUsed(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.db.Exec(`UPDATE api_key SET last_used = ? WHERE id = ?`, now, id)
	return err
}

func scanAPIKey(row *sql.Row) (*APIKeyRecord, error) {
	var r APIKeyRecord
	var enabled int
	var accountID, expiresAt, lastUsed sql.NullString
	err := row.Scan(&r.ID, &r.UserID, &accountID, &r.Name, &r.Prefix, &r.KeyHash, &expiresAt, &lastUsed, &enabled, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Enabled = enabled != 0
	if accountID.Valid {
		r.AccountID = &accountID.String
	}
	if expiresAt.Valid {
		r.ExpiresAt = &expiresAt.String
	}
	if lastUsed.Valid {
		r.LastUsed = &lastUsed.String
	}
	return &r, nil
}

func scanAPIKeyRow(rows *sql.Rows) (*APIKeyRecord, error) {
	var r APIKeyRecord
	var enabled int
	var accountID, expiresAt, lastUsed sql.NullString
	err := rows.Scan(&r.ID, &r.UserID, &accountID, &r.Name, &r.Prefix, &r.KeyHash, &expiresAt, &lastUsed, &enabled, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	r.Enabled = enabled != 0
	if accountID.Valid {
		r.AccountID = &accountID.String
	}
	if expiresAt.Valid {
		r.ExpiresAt = &expiresAt.String
	}
	if lastUsed.Valid {
		r.LastUsed = &lastUsed.String
	}
	return &r, nil
}

// ─── Password Reset Token CRUD ──────────────────────────────

// CreateResetToken stores a hashed password reset token.
func (d *DB) CreateResetToken(rec *ResetTokenRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Invalidate any existing unused tokens for this user
	d.db.Exec(`UPDATE password_reset_token SET used = 1 WHERE user_id = ? AND used = 0`, rec.UserID)

	_, err := d.db.Exec(
		`INSERT INTO password_reset_token (id, user_id, token_hash, expires_at) VALUES (?, ?, ?, ?)`,
		rec.ID, rec.UserID, rec.TokenHash, rec.ExpiresAt,
	)
	return err
}

// GetResetTokenByHash finds an unused, non-expired reset token by its hash.
func (d *DB) GetResetTokenByHash(tokenHash string) (*ResetTokenRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	row := d.db.QueryRow(
		`SELECT id, user_id, token_hash, expires_at, used, created_at FROM password_reset_token WHERE token_hash = ? AND used = 0`,
		tokenHash,
	)

	var r ResetTokenRecord
	var used int
	err := row.Scan(&r.ID, &r.UserID, &r.TokenHash, &r.ExpiresAt, &used, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Used = used != 0
	return &r, nil
}

// MarkResetTokenUsed marks a token as consumed.
func (d *DB) MarkResetTokenUsed(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`UPDATE password_reset_token SET used = 1 WHERE id = ?`, id)
	return err
}
