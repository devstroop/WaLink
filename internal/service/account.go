package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/devstroop/walink/internal/database"
	"github.com/devstroop/walink/internal/model"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	"google.golang.org/protobuf/proto"
)

// Account wraps a single whatsmeow client with lifecycle management.
type Account struct {
	mu sync.RWMutex

	ID          string
	PhoneNumber string
	AccountName string
	DataDir     string
	CreatedAt   time.Time

	Proxy        *ProxyConfig // nil = direct, set via PUT /accounts/{id}/proxy

	db           *database.DB
	client       *whatsmeow.Client
	container    *sqlstore.Container
	eventCh      chan any
}

// NewAccount constructs an Account (not yet connected).
func NewAccount(id, phone, name, dataDir string, createdAt time.Time, db *database.DB) *Account {
	return &Account{
		ID:           id,
		PhoneNumber:  phone,
		AccountName:  name,
		DataDir:      dataDir,
		CreatedAt:    createdAt,
		db:           db,
	}
}

// Connect initialises the whatsmeow client and connects to WhatsApp servers.
func (a *Account) Connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client != nil && a.client.IsConnected() {
		return nil
	}

	if err := a.prepareClient(ctx); err != nil {
		return err
	}

	if err := a.client.Connect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	log.Info().Str("account", a.ID).Msg("connected to WhatsApp")
	return nil
}

// prepareClient creates the whatsmeow client and store without connecting.
// Must be called with a.mu held.
func (a *Account) prepareClient(ctx context.Context) error {
	dbPath := filepath.Join(a.DataDir, "whatsmeow.db")
	if err := os.MkdirAll(a.DataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	logger := waLog.Noop
	container, err := sqlstore.New(ctx, "sqlite", fmt.Sprintf("file:%s?_pragma=foreign_keys(1)", dbPath), logger)
	if err != nil {
		return fmt.Errorf("open whatsmeow store: %w", err)
	}
	a.container = container

	// Get or create device
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("get device: %w", err)
	}

	client := whatsmeow.NewClient(device, logger)
	// Build HTTP transport — force IPv4, optionally route through proxy
	ipv4Dialer := &net.Dialer{}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return ipv4Dialer.DialContext(ctx, "tcp4", addr)
		},
		ForceAttemptHTTP2: true,
	}
	if a.Proxy != nil && a.Proxy.Enabled {
		proxyURL := a.Proxy.URL()
		transport.Proxy = http.ProxyURL(proxyURL)
		log.Info().Str("account", a.ID).Str("proxy", proxyURL.Host).Msg("using proxy")
	}
	client.SetWebsocketHTTPClient(&http.Client{Transport: transport})
	a.client = client

	// Event handler
	a.eventCh = make(chan any, 64)
	client.AddEventHandler(func(evt interface{}) {
		a.handleEvent(evt)
	})

	return nil
}

// Disconnect gracefully closes the whatsmeow connection.
func (a *Account) Disconnect() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client != nil {
		a.client.Disconnect()
		a.client = nil
	}
	log.Info().Str("account", a.ID).Msg("disconnected")
}

// EnsureConnected auto-connects if sleeping and waits until ready.
func (a *Account) EnsureConnected(ctx context.Context) error {
	a.mu.RLock()
	connected := a.client != nil && a.client.IsConnected()
	a.mu.RUnlock()

	if connected {
		return nil
	}

	return a.Connect(ctx)
}

// IsLoggedIn returns true if whatsmeow has a valid session.
func (a *Account) IsLoggedIn() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.client != nil && a.client.IsLoggedIn()
}

// GetQR sets up the client for QR linking. It disconnects any existing session,
// prepares a fresh client, obtains a QR channel, then connects.
// whatsmeow requires GetQRChannel to be called before Connect.
//
// The returned channel emits QR code events. The caller MUST drain the channel
// after reading the desired code (call DrainQR) so whatsmeow doesn't disconnect
// due to a full channel buffer.
func (a *Account) GetQR(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Tear down any existing connection so we can start fresh
	if a.client != nil {
		a.client.Disconnect()
		a.client = nil
	}

	if err := a.prepareClient(ctx); err != nil {
		return nil, err
	}

	if a.client.Store.ID != nil {
		return nil, fmt.Errorf("already logged in")
	}

	// Use a long-lived background context so the QR channel + connection
	// survive after the HTTP handler returns the QR PNG to the client.
	qrCtx, _ := context.WithTimeout(context.Background(), 2*time.Minute)
	ch, err := a.client.GetQRChannel(qrCtx)
	if err != nil {
		return nil, fmt.Errorf("get qr channel: %w", err)
	}

	if err := a.client.Connect(); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	return ch, nil
}

// DrainQR consumes remaining QR channel events in the background so
// whatsmeow doesn't disconnect due to a full channel buffer.
func DrainQR(ch <-chan whatsmeow.QRChannelItem) {
	go func() {
		for range ch {
		}
	}()
}

// PairPhone requests phone-number pairing and returns the linking code.
func (a *Account) PairPhone(ctx context.Context, phone string) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.client == nil {
		return "", fmt.Errorf("client not connected")
	}
	if a.client.Store.ID != nil {
		return "", fmt.Errorf("already logged in")
	}

	code, err := a.client.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		return "", fmt.Errorf("pair phone: %w", err)
	}
	return code, nil
}

// Logout logs out and clears the device store.
// If the client is connected, it sends a logout to WhatsApp servers first.
// If the client is nil (sleeping/already disconnected), it clears only local session data.
func (a *Account) Logout() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client != nil {
		// Connected: tell WhatsApp servers + clear local store
		err := a.client.Logout(context.Background())
		a.client.Disconnect()
		a.client = nil
		if err != nil {
			return fmt.Errorf("logout: %w", err)
		}
		return nil
	}

	// Not connected: just wipe local session data so
	// hasStoredSession() stops returning true.
	dbPath := filepath.Join(a.DataDir, "whatsmeow.db")
	if _, err := os.Stat(dbPath); err != nil {
		// No stored session at all — nothing to do
		return nil
	}
	// Open the store, grab the device, delete it
	logger := waLog.Noop
	container, err := sqlstore.New(context.Background(), "sqlite",
		fmt.Sprintf("file:%s?_pragma=foreign_keys(1)", dbPath), logger)
	if err != nil {
		return fmt.Errorf("open store for cleanup: %w", err)
	}
	device, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return fmt.Errorf("get device for cleanup: %w", err)
	}
	if device.ID != nil {
		if err := device.Delete(context.Background()); err != nil {
			return fmt.Errorf("delete stored device: %w", err)
		}
	}
	log.Info().Str("account", a.ID).Msg("cleared local session data (was not connected)")
	return nil
}

// SendMessage sends a text message to the given JID.
func (a *Account) SendMessage(ctx context.Context, jid string, text string) (string, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return "", fmt.Errorf("not connected")
	}

	target, err := types.ParseJID(jid)
	if err != nil {
		return "", fmt.Errorf("invalid jid %q: %w", jid, err)
	}

	msg := &waE2E.Message{
		Conversation: proto.String(text),
	}

	resp, err := client.SendMessage(ctx, target, msg)
	if err != nil {
		return "", fmt.Errorf("send: %w", err)
	}

	return resp.ID, nil
}

// SendMedia sends a document (file) with optional caption. Returns message ID.
func (a *Account) SendMedia(ctx context.Context, jid string, data []byte, filename, mimetype string, caption *string) (string, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return "", fmt.Errorf("not connected")
	}

	target, err := types.ParseJID(jid)
	if err != nil {
		return "", fmt.Errorf("invalid jid %q: %w", jid, err)
	}

	uploaded, err := client.Upload(ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		return "", fmt.Errorf("upload: %w", err)
	}

	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           &uploaded.URL,
			Mimetype:      &mimetype,
			FileName:      &filename,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			Caption:       caption,
		},
	}

	resp, err := client.SendMessage(ctx, target, msg)
	if err != nil {
		return "", fmt.Errorf("send media: %w", err)
	}

	return resp.ID, nil
}

// Info builds the API-facing AccountInfo.
func (a *Account) Info() model.AccountInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()

	info := model.AccountInfo{
		ID:          a.ID,
		AccountName: a.AccountName,
		Authorized:  a.hasStoredSession(),
		CreatedAt:   a.CreatedAt,
	}
	if a.PhoneNumber != "" {
		info.PhoneNumber = &a.PhoneNumber
	}
	return info
}

// StatusResponse builds a WhatsAppStatusResponse.
func (a *Account) StatusResponse() model.WhatsAppStatusResponse {
	a.mu.RLock()
	defer a.mu.RUnlock()

	resp := model.WhatsAppStatusResponse{
		AccountID:  a.ID,
		Authorized: a.hasStoredSession(),
	}
	if a.PhoneNumber != "" {
		resp.PhoneNumber = &a.PhoneNumber
	}
	return resp
}

// IsAuthorized reports whether the account has a valid WhatsApp session.
func (a *Account) IsAuthorized() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.hasStoredSession()
}

// HasStoredCredentials reports whether the on-disk whatsmeow store contains
// device credentials. This does NOT verify they are still valid on the server.
// Used to decide whether a connect-and-verify is worthwhile.
func (a *Account) HasStoredCredentials() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// If we have a live client, it knows
	if a.client != nil {
		return a.client.Store.ID != nil
	}
	// Probe disk
	dbPath := filepath.Join(a.DataDir, "whatsmeow.db")
	if _, err := os.Stat(dbPath); err != nil {
		return false
	}
	logger := waLog.Noop
	container, err := sqlstore.New(context.Background(), "sqlite",
		fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&mode=ro", dbPath), logger)
	if err != nil {
		return false
	}
	device, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return false
	}
	return device.ID != nil
}

// hasStoredSession checks whether there is a valid session from a live client.
// When the client is nil (sleeping), we cannot verify the session against the
// WhatsApp server, so we conservatively report false. Accounts with stored
// credentials are verified on startup via DiscoverAccounts, which connects
// them and lets whatsmeow detect revoked sessions via the LoggedOut event.
// Must be called with a.mu held.
func (a *Account) hasStoredSession() bool {
	if a.client != nil {
		return a.client.IsLoggedIn()
	}
	return false
}

// Reset clears all session data and re-creates the data directory.
func (a *Account) Reset() error {
	a.Disconnect()

	a.mu.Lock()
	defer a.mu.Unlock()

	if err := os.RemoveAll(a.DataDir); err != nil {
		return fmt.Errorf("remove data dir: %w", err)
	}
	if err := os.MkdirAll(a.DataDir, 0o755); err != nil {
		return fmt.Errorf("recreate data dir: %w", err)
	}
	return nil
}

// SendChatPresence sends a typing or paused indicator in a chat.
func (a *Account) SendChatPresence(ctx context.Context, jid string, state string) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}

	target, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("invalid jid %q: %w", jid, err)
	}

	var presence types.ChatPresence
	switch state {
	case "composing":
		presence = types.ChatPresenceComposing
	case "paused":
		presence = types.ChatPresencePaused
	default:
		return fmt.Errorf("invalid state %q: must be composing or paused", state)
	}

	if err := client.SendChatPresence(ctx, target, presence, types.ChatPresenceMediaText); err != nil {
		return fmt.Errorf("send presence: %w", err)
	}

	return nil
}

// MarkRead marks messages as read in a chat.
func (a *Account) MarkRead(ctx context.Context, chatJID string, messageIDs []string) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}

	target, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("invalid jid %q: %w", chatJID, err)
	}

	if err := client.MarkRead(ctx, messageIDs, time.Now(), target, types.JID{}); err != nil {
		return fmt.Errorf("mark read: %w", err)
	}

	return nil
}

// SendReaction sends an emoji reaction on a message.
func (a *Account) SendReaction(ctx context.Context, chatJID, messageID, emoji string) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}

	target, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("invalid jid %q: %w", chatJID, err)
	}

	// Use whatsmeow's BuildReaction helper — it handles MessageKey construction.
	msg := client.BuildReaction(target, types.EmptyJID, types.MessageID(messageID), emoji)

	if _, err := client.SendMessage(ctx, target, msg); err != nil {
		return fmt.Errorf("send reaction: %w", err)
	}

	return nil
}

// SendReply sends a text message quoting another message.
func (a *Account) SendReply(ctx context.Context, chatJID, messageID, text string) (string, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return "", fmt.Errorf("not connected")
	}

	target, err := types.ParseJID(chatJID)
	if err != nil {
		return "", fmt.Errorf("invalid jid %q: %w", chatJID, err)
	}

	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String(text),
			ContextInfo: &waE2E.ContextInfo{
				StanzaID:      proto.String(messageID),
				QuotedMessage: &waE2E.Message{},
			},
		},
	}

	resp, err := client.SendMessage(ctx, target, msg)
	if err != nil {
		return "", fmt.Errorf("send reply: %w", err)
	}

	return resp.ID, nil
}

// GetContactInfo returns contact details from the whatsmeow store.
func (a *Account) GetContactInfo(ctx context.Context, contactJID string) (model.ContactInfo, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return model.ContactInfo{}, fmt.Errorf("not connected")
	}

	jid, err := types.ParseJID(contactJID)
	if err != nil {
		return model.ContactInfo{}, fmt.Errorf("invalid jid %q: %w", contactJID, err)
	}

	info, err := client.Store.Contacts.GetContact(ctx, jid)
	if err != nil {
		return model.ContactInfo{}, fmt.Errorf("get contact: %w", err)
	}

	result := model.ContactInfo{
		ID:           contactJID,
		PushName:     info.PushName,
		FullName:     info.FullName,
		FirstName:    info.FirstName,
		BusinessName: info.BusinessName,
	}
	if jid.Server == types.DefaultUserServer {
		phone := jid.User
		result.Phone = &phone
	}

	// Profile picture (best-effort)
	pic, err := client.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{Preview: false})
	if err == nil && pic != nil {
		result.PictureURL = &pic.URL
	}

	return result, nil
}

// GetGroupInfo fetches group details from WhatsApp servers.
func (a *Account) GetGroupInfo(ctx context.Context, groupJID string) (model.GroupInfo, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return model.GroupInfo{}, fmt.Errorf("not connected")
	}

	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return model.GroupInfo{}, fmt.Errorf("invalid jid %q: %w", groupJID, err)
	}

	gi, err := client.GetGroupInfo(ctx, jid)
	if err != nil {
		return model.GroupInfo{}, fmt.Errorf("get group info: %w", err)
	}

	return groupInfoToModel(gi), nil
}

// ListGroups returns all joined groups.
func (a *Account) ListGroups(ctx context.Context) ([]model.GroupInfo, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	groups, err := client.GetJoinedGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("get joined groups: %w", err)
	}

	result := make([]model.GroupInfo, len(groups))
	for i, gi := range groups {
		result[i] = groupInfoToModel(gi)
	}
	return result, nil
}

// CreateGroup creates a new WhatsApp group.
func (a *Account) CreateGroup(ctx context.Context, name string, participants []string) (model.GroupInfo, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return model.GroupInfo{}, fmt.Errorf("not connected")
	}

	jids := make([]types.JID, len(participants))
	for i, p := range participants {
		j, err := types.ParseJID(p)
		if err != nil {
			return model.GroupInfo{}, fmt.Errorf("invalid participant jid %q: %w", p, err)
		}
		jids[i] = j
	}

	gi, err := client.CreateGroup(ctx, whatsmeow.ReqCreateGroup{
		Name:         name,
		Participants: jids,
	})
	if err != nil {
		return model.GroupInfo{}, fmt.Errorf("create group: %w", err)
	}

	return groupInfoToModel(gi), nil
}

// LeaveGroup leaves a WhatsApp group.
func (a *Account) LeaveGroup(ctx context.Context, groupJID string) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}

	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return fmt.Errorf("invalid jid %q: %w", groupJID, err)
	}

	return client.LeaveGroup(ctx, jid)
}

// UpdateGroup updates group settings (name, description, locked, announce).
func (a *Account) UpdateGroup(ctx context.Context, groupJID string, req model.UpdateGroupRequest) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}

	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return fmt.Errorf("invalid jid %q: %w", groupJID, err)
	}

	if req.Name != nil {
		if err := client.SetGroupName(ctx, jid, *req.Name); err != nil {
			return fmt.Errorf("set group name: %w", err)
		}
	}
	if req.Description != nil {
		if err := client.SetGroupDescription(ctx, jid, *req.Description); err != nil {
			return fmt.Errorf("set group description: %w", err)
		}
	}
	if req.Locked != nil {
		if err := client.SetGroupLocked(ctx, jid, *req.Locked); err != nil {
			return fmt.Errorf("set group locked: %w", err)
		}
	}
	if req.Announce != nil {
		if err := client.SetGroupAnnounce(ctx, jid, *req.Announce); err != nil {
			return fmt.Errorf("set group announce: %w", err)
		}
	}
	return nil
}

// UpdateGroupParticipants adds/removes/promotes/demotes group members.
func (a *Account) UpdateGroupParticipants(ctx context.Context, groupJID string, participants []string, action string) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}

	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return fmt.Errorf("invalid jid %q: %w", groupJID, err)
	}

	jids := make([]types.JID, len(participants))
	for i, p := range participants {
		j, err := types.ParseJID(p)
		if err != nil {
			return fmt.Errorf("invalid participant jid %q: %w", p, err)
		}
		jids[i] = j
	}

	var change whatsmeow.ParticipantChange
	switch action {
	case "add":
		change = whatsmeow.ParticipantChangeAdd
	case "remove":
		change = whatsmeow.ParticipantChangeRemove
	case "promote":
		change = whatsmeow.ParticipantChangePromote
	case "demote":
		change = whatsmeow.ParticipantChangeDemote
	default:
		return fmt.Errorf("invalid action %q: must be add, remove, promote, or demote", action)
	}

	_, err = client.UpdateGroupParticipants(ctx, jid, jids, change)
	return err
}

// GetGroupInviteLink returns the group's invite link.
func (a *Account) GetGroupInviteLink(ctx context.Context, groupJID string, reset bool) (string, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return "", fmt.Errorf("not connected")
	}

	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return "", fmt.Errorf("invalid jid %q: %w", groupJID, err)
	}

	return client.GetGroupInviteLink(ctx, jid, reset)
}

// ── Presence ────────────────────────────────────────

// SendPresence sets global online/offline presence.
func (a *Account) SendPresence(ctx context.Context, state string) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}

	var p types.Presence
	switch state {
	case "available":
		p = types.PresenceAvailable
	case "unavailable":
		p = types.PresenceUnavailable
	default:
		return fmt.Errorf("invalid state %q: must be available or unavailable", state)
	}
	return client.SendPresence(ctx, p)
}

// ── Profile ─────────────────────────────────────────

// GetProfile returns the account's own profile info.
func (a *Account) GetProfile(ctx context.Context) (model.ProfileResponse, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return model.ProfileResponse{}, fmt.Errorf("not connected")
	}

	resp := model.ProfileResponse{ID: a.ID}
	if a.PhoneNumber != "" {
		resp.PhoneNumber = &a.PhoneNumber
	}

	if client.Store.ID != nil {
		ownJID := *client.Store.ID

		// Profile picture
		pic, err := client.GetProfilePictureInfo(ctx, ownJID, &whatsmeow.GetProfilePictureParams{Preview: false})
		if err == nil && pic != nil {
			resp.PictureURL = &pic.URL
		}

		// About / status text via GetUserInfo
		userInfo, err := client.GetUserInfo(ctx, []types.JID{ownJID})
		if err == nil {
			if info, ok := userInfo[ownJID]; ok && info.Status != "" {
				resp.About = &info.Status
			}
		}
	}

	return resp, nil
}

// SetStatusMessage sets the "About" text.
func (a *Account) SetStatusMessage(ctx context.Context, about string) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}

	return client.SetStatusMessage(ctx, about)
}

// ── helpers ─────────────────────────────────────────

func groupInfoToModel(gi *types.GroupInfo) model.GroupInfo {
	participants := make([]model.GroupParticipant, len(gi.Participants))
	for i, p := range gi.Participants {
		gp := model.GroupParticipant{
			ID:      p.JID.String(),
			IsAdmin: p.IsAdmin || p.IsSuperAdmin,
		}
		if p.DisplayName != "" {
			gp.Name = &p.DisplayName
		}
		participants[i] = gp
	}

	created := gi.GroupCreated.Format(time.RFC3339)
	owner := gi.OwnerJID.String()

	result := model.GroupInfo{
		ID:               gi.JID.String(),
		Name:             gi.Name,
		ParticipantCount: len(gi.Participants),
		Participants:     participants,
		IsAnnounce:       gi.IsAnnounce,
		IsLocked:         gi.IsLocked,
		CreatedAt:        &created,
		CreatedBy:        &owner,
	}
	if gi.Topic != "" {
		result.Description = &gi.Topic
	}
	return result
}

// ListContacts returns all contacts from the whatsmeow store.
func (a *Account) ListContacts(ctx context.Context) ([]model.ContactInfo, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("not connected")
	}

	contacts, err := client.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get contacts: %w", err)
	}

	result := make([]model.ContactInfo, 0, len(contacts))
	for jid, info := range contacts {
		ci := model.ContactInfo{
			ID:           jid.String(),
			PushName:     info.PushName,
			FullName:     info.FullName,
			FirstName:    info.FirstName,
			BusinessName: info.BusinessName,
		}
		if jid.Server == types.DefaultUserServer {
			phone := jid.User
			ci.Phone = &phone
		}
		result = append(result, ci)
	}
	return result, nil
}

// CheckContacts checks which phone numbers are registered on WhatsApp.
func (a *Account) CheckContacts(ctx context.Context, phones []string) ([]model.CheckContactResult, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	resp, err := client.IsOnWhatsApp(ctx, phones)
	if err != nil {
		return nil, fmt.Errorf("check contacts: %w", err)
	}

	results := make([]model.CheckContactResult, len(resp))
	for i, r := range resp {
		results[i] = model.CheckContactResult{
			Phone:      r.Query,
			OnWhatsApp: r.IsIn,
		}
		if r.IsIn {
			results[i].JID = r.JID.String()
		}
	}
	return results, nil
}

// RevokeMessage revokes (deletes for everyone) a previously sent message.
func (a *Account) RevokeMessage(ctx context.Context, chatJID, messageID string) (model.RevokeMessageResponse, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return model.RevokeMessageResponse{}, fmt.Errorf("not connected")
	}

	target, err := types.ParseJID(chatJID)
	if err != nil {
		return model.RevokeMessageResponse{}, fmt.Errorf("invalid jid %q: %w", chatJID, err)
	}

	resp, err := client.RevokeMessage(ctx, target, types.MessageID(messageID))
	if err != nil {
		return model.RevokeMessageResponse{}, fmt.Errorf("revoke: %w", err)
	}

	return model.RevokeMessageResponse{
		Revoked:   true,
		Timestamp: resp.Timestamp.Format(time.RFC3339),
	}, nil
}

// ListMessages returns stored messages for a chat with cursor pagination.
func (a *Account) ListMessages(chatJID string, limit int, before string) (model.MessageListResponse, error) {
	if a.db == nil {
		return model.MessageListResponse{}, fmt.Errorf("no database")
	}
	records, err := a.db.ListMessages(a.ID, chatJID, limit, before)
	if err != nil {
		return model.MessageListResponse{}, fmt.Errorf("list messages: %w", err)
	}
	msgs := make([]model.MessageInfo, len(records))
	for i, r := range records {
		msgs[i] = model.MessageInfo{
			ID:        r.ID,
			ChatJID:   r.ChatJID,
			SenderJID: r.SenderJID,
			FromMe:    r.FromMe,
			Type:      r.Type,
			Body:      r.Body,
			MediaType: r.MediaType,
			Timestamp: r.Timestamp,
		}
	}
	return model.MessageListResponse{
		Messages: msgs,
		Count:    len(msgs),
	}, nil
}

// DownloadMedia downloads media from a received message using whatsmeow.
func (a *Account) DownloadMedia(ctx context.Context, msg *waE2E.Message) ([]byte, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	data, err := client.DownloadAny(ctx, msg)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	return data, nil
}

// ListChats returns known contacts and groups from the whatsmeow store.
func (a *Account) ListChats(ctx context.Context) ([]model.ChatInfo, error) {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("not connected")
	}

	seen := make(map[string]bool)
	var chats []model.ChatInfo

	// Helper to enrich a chat entry with local settings (pinned/muted/archived)
	enrich := func(chat *model.ChatInfo, jid types.JID) {
		settings, err := client.Store.ChatSettings.GetChatSettings(ctx, jid)
		if err == nil && settings.Found {
			chat.Pinned = settings.Pinned
			chat.Archived = settings.Archived
			chat.Muted = !settings.MutedUntil.IsZero()
		}
	}

	// 1. Groups from server
	groups, err := client.GetJoinedGroups(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to fetch groups from server")
	} else {
		for _, g := range groups {
			id := g.JID.String()
			seen[id] = true
			chat := model.ChatInfo{
				ID:      id,
				Name:    g.GroupName.Name,
				IsGroup: true,
			}
			enrich(&chat, g.JID)
			chats = append(chats, chat)
		}
	}

	// 2. Contacts from local store (populated via history sync)
	contacts, err := client.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to fetch contacts from store")
	} else {
		for jid, info := range contacts {
			id := jid.String()
			if seen[id] {
				continue
			}
			name := info.PushName
			if info.FullName != "" {
				name = info.FullName
			}
			if info.BusinessName != "" {
				name = info.BusinessName
			}
			if name == "" {
				name = jid.User
			}
			chat := model.ChatInfo{
				ID:      id,
				Name:    name,
				IsGroup: jid.Server == types.GroupServer,
			}
			enrich(&chat, jid)
			chats = append(chats, chat)
		}
	}

	return chats, nil
}

// ---- internal ----

func (a *Account) handleEvent(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		log.Debug().Str("account", a.ID).Str("from", v.Info.Sender.String()).Msg("message received")
		a.storeMessage(v)
		a.dispatchWebhook("message", map[string]any{
			"id":         v.Info.ID,
			"chat":       v.Info.Chat.String(),
			"sender":     v.Info.Sender.String(),
			"from_me":    v.Info.IsFromMe,
			"type":       classifyMessage(v.Message),
			"body":       extractBody(v.Message),
			"media_type": v.Info.MediaType,
			"timestamp":  v.Info.Timestamp.UTC().Format(time.RFC3339),
		})
	case *events.Receipt:
		log.Debug().Str("account", a.ID).Str("type", string(v.Type)).Int("count", len(v.MessageIDs)).Msg("receipt")
		a.dispatchWebhook("receipt", map[string]any{
			"type":        string(v.Type),
			"chat":        v.Chat.String(),
			"sender":      v.Sender.String(),
			"message_ids": v.MessageIDs,
			"timestamp":   v.Timestamp.UTC().Format(time.RFC3339),
		})
	case *events.PushName:
		log.Debug().Str("account", a.ID).Str("jid", v.JID.String()).Str("name", v.NewPushName).Msg("push name update")
	case *events.HistorySync:
		log.Info().Str("account", a.ID).Msg("history sync received")
	case *events.Connected:
		log.Info().Str("account", a.ID).Msg("whatsmeow connected event")
	case *events.LoggedOut:
		log.Warn().Str("account", a.ID).Int("reason", int(v.Reason)).Msg("logged out by phone — cleaning up")
		a.mu.Lock()
		if a.client != nil {
			a.client.Disconnect()
			a.client = nil
		}
		a.mu.Unlock()
	case *events.Disconnected:
		log.Warn().Str("account", a.ID).Msg("disconnected event")
	}
}

// storeMessage persists a received message to the DB.
func (a *Account) storeMessage(v *events.Message) {
	if a.db == nil {
		return
	}
	rec := &database.MessageRecord{
		ID:        v.Info.ID,
		AccountID: a.ID,
		ChatJID:   v.Info.Chat.String(),
		SenderJID: v.Info.Sender.String(),
		FromMe:    v.Info.IsFromMe,
		Type:      classifyMessage(v.Message),
		Body:      extractBody(v.Message),
		MediaType: v.Info.MediaType,
		Timestamp: v.Info.Timestamp.UTC().Format(time.RFC3339),
	}
	if err := a.db.InsertMessage(rec); err != nil {
		log.Warn().Str("account", a.ID).Err(err).Msg("failed to store message")
	}
}

// classifyMessage returns a short type label for the message.
func classifyMessage(msg *waE2E.Message) string {
	if msg == nil {
		return "unknown"
	}
	switch {
	case msg.Conversation != nil || msg.ExtendedTextMessage != nil:
		return "text"
	case msg.ImageMessage != nil:
		return "image"
	case msg.VideoMessage != nil:
		return "video"
	case msg.AudioMessage != nil:
		return "audio"
	case msg.DocumentMessage != nil:
		return "document"
	case msg.StickerMessage != nil:
		return "sticker"
	case msg.ReactionMessage != nil:
		return "reaction"
	case msg.ContactMessage != nil:
		return "contact"
	case msg.LocationMessage != nil:
		return "location"
	default:
		return "other"
	}
}

// extractBody returns the textual content of a message.
func extractBody(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	switch {
	case msg.Conversation != nil:
		return msg.GetConversation()
	case msg.ExtendedTextMessage != nil:
		return msg.ExtendedTextMessage.GetText()
	case msg.ImageMessage != nil:
		return msg.ImageMessage.GetCaption()
	case msg.VideoMessage != nil:
		return msg.VideoMessage.GetCaption()
	case msg.DocumentMessage != nil:
		return msg.DocumentMessage.GetCaption()
	case msg.ReactionMessage != nil:
		return msg.ReactionMessage.GetText()
	default:
		return ""
	}
}

// dispatchWebhook sends an event to the account's webhook URL, if configured.
func (a *Account) dispatchWebhook(eventType string, payload map[string]any) {
	if a.db == nil {
		return
	}
	go a.doDispatchWebhook(eventType, payload)
}

func (a *Account) doDispatchWebhook(eventType string, payload map[string]any) {
	cfg, err := a.db.GetWebhookConfig(a.ID)
	if err != nil || cfg == nil || !cfg.Enabled || cfg.URL == "" {
		return
	}

	// Filter by event type if events are specified
	if cfg.Events != "" {
		allowed := false
		for _, e := range splitCSV(cfg.Events) {
			if e == eventType {
				allowed = true
				break
			}
		}
		if !allowed {
			return
		}
	}

	evt := model.WebhookEvent{
		EventType: eventType,
		AccountID: a.ID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Payload:   payload,
	}

	body, err := json.Marshal(evt)
	if err != nil {
		log.Warn().Str("account", a.ID).Err(err).Msg("webhook: marshal failed")
		return
	}

	req, err := http.NewRequest("POST", cfg.URL, bytes.NewReader(body))
	if err != nil {
		log.Warn().Str("account", a.ID).Err(err).Msg("webhook: create request failed")
		return
	}
	req.Header.Set("Content-Type", "application/json")

	// HMAC signature if secret is set
	if cfg.Secret != "" {
		mac := hmac.New(sha256.New, []byte(cfg.Secret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Webhook-Signature", sig)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Warn().Str("account", a.ID).Err(err).Msg("webhook: POST failed")
		return
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Warn().Str("account", a.ID).Int("status", resp.StatusCode).Msg("webhook: non-success response")
	}
}

// splitCSV splits a comma-separated string into trimmed parts.
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---- phone helpers ----

// NormalizePhone strips non-digit chars and ensures a clean E.164-ish string.
func NormalizePhone(phone string) string {
	var out []byte
	for _, c := range []byte(phone) {
		if c >= '0' && c <= '9' {
			out = append(out, c)
		}
	}
	return string(out)
}

// PhoneToJID converts a phone number to a WhatsApp JID.
func PhoneToJID(phone string) string {
	p := NormalizePhone(phone)
	return p + "@s.whatsapp.net"
}

// NewUUID generates a new UUID string.
func NewUUID() string { return uuid.New().String() }
