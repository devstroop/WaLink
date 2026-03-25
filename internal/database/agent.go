package database

import (
	"database/sql"
	"errors"
	"time"
)

// AgentSessionRecord stores the chat history for a user's assistant session.
type AgentSessionRecord struct {
	UserID    string
	Messages  string // JSON array of agent.Message
	UpdatedAt string // RFC3339
}

// AgentConfigRecord stores per-account autopilot settings.
type AgentConfigRecord struct {
	AccountID          string
	Enabled            bool
	SystemPrompt       string
	Model              string // override global model (empty = use global default)
	EscalationEnabled  bool   // if true, AI detects escalation intent and sends a handoff message
	EscalationMessage  string // custom message sent on escalation (empty = use default)
	Whitelist          string // newline-separated phone numbers; if non-empty only these get replies
	Blacklist          string // newline-separated phone numbers; these are always skipped
	UpdatedAt          string
}

// AgentLogRecord is a single autopilot auto-reply audit entry.
type AgentLogRecord struct {
	ID              string
	AccountID       string
	ChatJID         string
	SenderJID       string
	IncomingMessage string
	OutgoingMessage string
	Model           string
	CreatedAt       string
}

// migrateAgent creates the agent tables (called from migrate()).
func (d *DB) migrateAgent() error {
	_, err := d.db.Exec(`
		CREATE TABLE IF NOT EXISTS agent_session (
			user_id    TEXT PRIMARY KEY,
			messages   TEXT NOT NULL DEFAULT '[]',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(`
		CREATE TABLE IF NOT EXISTS agent_config (
			account_id           TEXT PRIMARY KEY,
			enabled              BOOLEAN NOT NULL DEFAULT FALSE,
			system_prompt        TEXT NOT NULL DEFAULT '',
			model                TEXT NOT NULL DEFAULT '',
			escalation_enabled   BOOLEAN NOT NULL DEFAULT FALSE,
			escalation_message   TEXT NOT NULL DEFAULT '',
			whitelist            TEXT NOT NULL DEFAULT '',
			blacklist            TEXT NOT NULL DEFAULT '',
			updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return err
	}

	// Add columns to existing tables (idempotent).
	_, err = d.db.Exec(`ALTER TABLE agent_config ADD COLUMN IF NOT EXISTS whitelist TEXT NOT NULL DEFAULT ''`)
	if err != nil {
		return err
	}
	_, err = d.db.Exec(`ALTER TABLE agent_config ADD COLUMN IF NOT EXISTS blacklist TEXT NOT NULL DEFAULT ''`)
	if err != nil {
		return err
	}
	_, err = d.db.Exec(`ALTER TABLE agent_config ADD COLUMN IF NOT EXISTS escalation_enabled BOOLEAN NOT NULL DEFAULT FALSE`)
	if err != nil {
		return err
	}
	_, err = d.db.Exec(`ALTER TABLE agent_config ADD COLUMN IF NOT EXISTS escalation_message TEXT NOT NULL DEFAULT ''`)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(`
		CREATE TABLE IF NOT EXISTS agent_log (
			id               TEXT PRIMARY KEY,
			account_id       TEXT NOT NULL,
			chat_jid         TEXT NOT NULL,
			sender_jid       TEXT NOT NULL DEFAULT '',
			incoming_message TEXT NOT NULL DEFAULT '',
			outgoing_message TEXT NOT NULL DEFAULT '',
			model            TEXT NOT NULL DEFAULT '',
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_agent_log_account ON agent_log (account_id, created_at DESC);`,
	)
	return err
}

// ── Session ──────────────────────────────────────────────────────────────────

// GetAgentSession returns the stored message history JSON for a user.
// Returns ("[]", nil) when no session exists yet.
func (d *DB) GetAgentSession(userID string) (string, error) {
	var msgs string
	err := d.db.QueryRow(`SELECT messages FROM agent_session WHERE user_id = $1`, userID).Scan(&msgs)
	if errors.Is(err, sql.ErrNoRows) {
		return "[]", nil
	}
	return msgs, err
}

// SaveAgentSession upserts the message history JSON for a user.
func (d *DB) SaveAgentSession(userID, messagesJSON string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.db.Exec(`
		INSERT INTO agent_session (user_id, messages, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT(user_id) DO UPDATE SET messages = excluded.messages, updated_at = excluded.updated_at
	`, userID, messagesJSON, now)
	return err
}

// ClearAgentSession deletes the stored message history for a user.
func (d *DB) ClearAgentSession(userID string) error {
	_, err := d.db.Exec(`DELETE FROM agent_session WHERE user_id = $1`, userID)
	return err
}

// ── Config ───────────────────────────────────────────────────────────────────

// GetAgentConfig returns the autopilot configuration for an account.
// Returns a default (disabled) config when none is stored.
func (d *DB) GetAgentConfig(accountID string) (*AgentConfigRecord, error) {
	row := &AgentConfigRecord{AccountID: accountID}
	err := d.db.QueryRow(`
		SELECT enabled, system_prompt, model, escalation_enabled, escalation_message, whitelist, blacklist, updated_at
		FROM agent_config WHERE account_id = $1
	`, accountID).Scan(&row.Enabled, &row.SystemPrompt, &row.Model, &row.EscalationEnabled, &row.EscalationMessage, &row.Whitelist, &row.Blacklist, &row.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return row, nil
	}
	return row, err
}

// SetAgentConfig upserts the autopilot configuration for an account.
func (d *DB) SetAgentConfig(cfg AgentConfigRecord) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.db.Exec(`
		INSERT INTO agent_config (account_id, enabled, system_prompt, model, escalation_enabled, escalation_message, whitelist, blacklist, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT(account_id) DO UPDATE SET
			enabled             = excluded.enabled,
			system_prompt       = excluded.system_prompt,
			model               = excluded.model,
			escalation_enabled  = excluded.escalation_enabled,
			escalation_message  = excluded.escalation_message,
			whitelist           = excluded.whitelist,
			blacklist           = excluded.blacklist,
			updated_at          = excluded.updated_at
	`, cfg.AccountID, cfg.Enabled, cfg.SystemPrompt, cfg.Model, cfg.EscalationEnabled, cfg.EscalationMessage, cfg.Whitelist, cfg.Blacklist, now)
	return err
}

// ── Logs ─────────────────────────────────────────────────────────────────────

// InsertAgentLog appends a new auto-reply log entry.
func (d *DB) InsertAgentLog(rec AgentLogRecord) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.db.Exec(`
		INSERT INTO agent_log (id, account_id, chat_jid, sender_jid, incoming_message, outgoing_message, model, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, rec.ID, rec.AccountID, rec.ChatJID, rec.SenderJID, rec.IncomingMessage, rec.OutgoingMessage, rec.Model, now)
	return err
}

// ListAgentLogs returns the most recent log entries for an account.
func (d *DB) ListAgentLogs(accountID string, limit int) ([]AgentLogRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := d.db.Query(`
		SELECT id, account_id, chat_jid, sender_jid, incoming_message, outgoing_message, model, created_at
		FROM agent_log WHERE account_id = $1
		ORDER BY created_at DESC LIMIT $2
	`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []AgentLogRecord
	for rows.Next() {
		var r AgentLogRecord
		if err := rows.Scan(&r.ID, &r.AccountID, &r.ChatJID, &r.SenderJID,
			&r.IncomingMessage, &r.OutgoingMessage, &r.Model, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListAllEnabledAgentConfigs returns all accounts that have autopilot enabled.
func (d *DB) ListAllEnabledAgentConfigs() ([]AgentConfigRecord, error) {
	rows, err := d.db.Query(`
		SELECT account_id, enabled, system_prompt, model, escalation_enabled, escalation_message, whitelist, blacklist, updated_at
		FROM agent_config WHERE enabled = TRUE
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []AgentConfigRecord
	for rows.Next() {
		var r AgentConfigRecord
		if err := rows.Scan(&r.AccountID, &r.Enabled, &r.SystemPrompt, &r.Model, &r.EscalationEnabled, &r.EscalationMessage, &r.Whitelist, &r.Blacklist, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
