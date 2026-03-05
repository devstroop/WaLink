package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/itsalfredakku/walink/internal/model"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
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
	IdleTimeout int64 // seconds; 0 = never sleep
	Status      model.AccountStatus
	CreatedAt   time.Time

	client       *whatsmeow.Client
	container    *sqlstore.Container
	lastActivity time.Time
	idleCancel   context.CancelFunc
	eventCh      chan any
}

// NewAccount constructs an Account (not yet connected).
func NewAccount(id, phone, name, dataDir string, idleTimeout int64, createdAt time.Time) *Account {
	return &Account{
		ID:           id,
		PhoneNumber:  phone,
		AccountName:  name,
		DataDir:      dataDir,
		IdleTimeout:  idleTimeout,
		Status:       model.StatusSleeping,
		CreatedAt:    createdAt,
		lastActivity: time.Now(),
	}
}

// Connect initialises the whatsmeow client and connects to WhatsApp servers.
func (a *Account) Connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.Status == model.StatusActive && a.client != nil && a.client.IsConnected() {
		return nil
	}

	if err := a.prepareClient(ctx); err != nil {
		return err
	}

	if err := a.client.Connect(); err != nil {
		a.Status = model.StatusError
		return fmt.Errorf("connect: %w", err)
	}

	a.Status = model.StatusActive
	a.lastActivity = time.Now()
	a.startIdleTimer()

	log.Info().Str("account", a.ID).Msg("connected to WhatsApp")
	return nil
}

// prepareClient creates the whatsmeow client and store without connecting.
// Must be called with a.mu held.
func (a *Account) prepareClient(ctx context.Context) error {
	a.Status = model.StatusConnecting

	dbPath := filepath.Join(a.DataDir, "whatsmeow.db")
	if err := os.MkdirAll(a.DataDir, 0o755); err != nil {
		a.Status = model.StatusError
		return fmt.Errorf("create data dir: %w", err)
	}

	logger := waLog.Noop
	container, err := sqlstore.New(ctx, "sqlite", fmt.Sprintf("file:%s?_pragma=foreign_keys(1)", dbPath), logger)
	if err != nil {
		a.Status = model.StatusError
		return fmt.Errorf("open whatsmeow store: %w", err)
	}
	a.container = container

	// Get or create device
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		a.Status = model.StatusError
		return fmt.Errorf("get device: %w", err)
	}

	client := whatsmeow.NewClient(device, logger)
	// Force IPv4 to avoid Windows IPv6 socket permission errors
	ipv4Dialer := &net.Dialer{}
	client.SetWebsocketHTTPClient(&http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return ipv4Dialer.DialContext(ctx, "tcp4", addr)
			},
			ForceAttemptHTTP2: true,
		},
	})
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

	a.stopIdleTimer()

	if a.client != nil {
		a.client.Disconnect()
		a.client = nil
	}
	a.Status = model.StatusSleeping
	log.Info().Str("account", a.ID).Msg("disconnected")
}

// EnsureConnected auto-connects if sleeping and waits until ready.
func (a *Account) EnsureConnected(ctx context.Context) error {
	a.mu.RLock()
	status := a.Status
	connected := a.client != nil && a.client.IsConnected()
	a.mu.RUnlock()

	if status == model.StatusActive && connected {
		a.TouchActivity()
		return nil
	}

	// Dead connection — reconnect
	if status == model.StatusActive && !connected {
		a.Disconnect()
	}

	return a.Connect(ctx)
}

// TouchActivity resets the idle timer.
func (a *Account) TouchActivity() {
	a.mu.Lock()
	a.lastActivity = time.Now()
	a.mu.Unlock()
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

	ch, err := a.client.GetQRChannel(ctx)
	if err != nil {
		return nil, fmt.Errorf("get qr channel: %w", err)
	}

	if err := a.client.Connect(); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	a.Status = model.StatusActive
	a.lastActivity = time.Now()
	a.startIdleTimer()

	return ch, nil
}

// PairPhone requests phone-number pairing and returns the linking code.
func (a *Account) PairPhone(ctx context.Context, phone string) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.client == nil {
		return "", fmt.Errorf("client not connected")
	}
	if a.client.IsLoggedIn() {
		return "", fmt.Errorf("already logged in")
	}

	code, err := a.client.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		return "", fmt.Errorf("pair phone: %w", err)
	}
	return code, nil
}

// Logout logs out and clears the device store.
func (a *Account) Logout() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client == nil {
		return fmt.Errorf("not connected")
	}
	err := a.client.Logout(context.Background())
	if err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	a.client.Disconnect()
	a.client = nil
	a.Status = model.StatusSleeping
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

	a.TouchActivity()
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

	a.TouchActivity()
	return resp.ID, nil
}

// Info builds the API-facing AccountInfo.
func (a *Account) Info() model.AccountInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()

	info := model.AccountInfo{
		ID:          a.ID,
		AccountName: a.AccountName,
		Status:      a.Status,
		Authorized:  a.client != nil && a.client.IsLoggedIn(),
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
		Status:     string(a.Status),
		Authorized: a.client != nil && a.client.IsLoggedIn(),
	}
	if a.PhoneNumber != "" {
		resp.PhoneNumber = &a.PhoneNumber
	}
	return resp
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
	a.Status = model.StatusSleeping
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

	a.TouchActivity()
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

	a.TouchActivity()
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

	msg := &waE2E.Message{
		ReactionMessage: &waE2E.ReactionMessage{
			Key: &waCommon.MessageKey{
				RemoteJID: proto.String(chatJID),
				FromMe:    proto.Bool(false),
				ID:        proto.String(messageID),
			},
			Text:              proto.String(emoji),
			SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
		},
	}

	if _, err := client.SendMessage(ctx, target, msg); err != nil {
		return fmt.Errorf("send reaction: %w", err)
	}

	a.TouchActivity()
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

	a.TouchActivity()
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

	result := model.ContactInfo{ID: contactJID}
	if info.FullName != "" {
		result.Name = &info.FullName
	}
	if info.PushName != "" {
		result.PushName = &info.PushName
	}
	if info.BusinessName != "" {
		result.IsBusiness = true
	}

	a.TouchActivity()
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

	a.TouchActivity()
	return result, nil
}

// ListChats returns known contacts as chat entries from the whatsmeow store.
func (a *Account) ListChats(ctx context.Context) ([]model.ChatInfo, error) {
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

	chats := make([]model.ChatInfo, 0, len(contacts))
	for jid, info := range contacts {
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
		chats = append(chats, model.ChatInfo{
			ID:      jid.String(),
			Name:    name,
			IsGroup: jid.Server == types.GroupServer,
		})
	}

	a.TouchActivity()
	return chats, nil
}

// ---- internal ----

func (a *Account) handleEvent(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		log.Debug().Str("account", a.ID).Str("from", v.Info.Sender.String()).Msg("message received")
	case *events.Receipt:
		log.Debug().Str("account", a.ID).Str("type", string(v.Type)).Int("count", len(v.MessageIDs)).Msg("receipt")
	case *events.PushName:
		log.Debug().Str("account", a.ID).Str("jid", v.JID.String()).Str("name", v.NewPushName).Msg("push name update")
	case *events.HistorySync:
		log.Info().Str("account", a.ID).Msg("history sync received")
	case *events.Connected:
		log.Info().Str("account", a.ID).Msg("whatsmeow connected event")
	case *events.LoggedOut:
		log.Warn().Str("account", a.ID).Int("reason", int(v.Reason)).Msg("logged out")
		a.mu.Lock()
		a.Status = model.StatusSleeping
		a.mu.Unlock()
	case *events.Disconnected:
		log.Warn().Str("account", a.ID).Msg("disconnected event")
	}
}

func (a *Account) startIdleTimer() {
	if a.IdleTimeout <= 0 {
		return
	}
	a.stopIdleTimer()

	ctx, cancel := context.WithCancel(context.Background())
	a.idleCancel = cancel

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.mu.RLock()
				idle := time.Since(a.lastActivity)
				timeout := time.Duration(a.IdleTimeout) * time.Second
				a.mu.RUnlock()

				if idle >= timeout {
					log.Info().Str("account", a.ID).Dur("idle", idle).Msg("idle timeout → disconnecting")
					a.Disconnect()
					return
				}
			}
		}
	}()
}

func (a *Account) stopIdleTimer() {
	if a.idleCancel != nil {
		a.idleCancel()
		a.idleCancel = nil
	}
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
