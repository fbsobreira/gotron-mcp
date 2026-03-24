package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Telegram Bot API method paths.
const (
	methodSendMessage         = "/sendMessage"
	methodEditMessageText     = "/editMessageText"
	methodGetUpdates          = "/getUpdates"
	methodAnswerCallbackQuery = "/answerCallbackQuery"

	maxResponseBody = 1 << 20 // 1 MB max response from Telegram API
	defaultTimeout  = 300     // 5 minutes
	pollTimeout     = 30      // seconds for long-poll
	pollRetryDelay  = 5 * time.Second
)

// TelegramConfig holds configuration for the Telegram approval backend.
type TelegramConfig struct {
	BotToken        string  // Telegram bot token from @BotFather
	AuthorizedUsers []int64 // Telegram user IDs allowed to approve
	ChatID          int64   // Chat ID to send approval messages to
	TimeoutSeconds  int     // Approval timeout (default 300 = 5 minutes)
}

// TelegramApprover implements Approver using the Telegram Bot API.
type TelegramApprover struct {
	cfg     TelegramConfig
	client  *http.Client
	baseURL string

	mu       sync.Mutex
	pending  map[string]chan Result // approval ID → result channel
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewTelegramApprover creates a Telegram approval backend and starts polling for updates.
func NewTelegramApprover(cfg TelegramConfig) (*TelegramApprover, error) {
	if cfg.BotToken == "" {
		return nil, fmt.Errorf("telegram bot token is required")
	}
	if cfg.ChatID == 0 {
		return nil, fmt.Errorf("telegram chat ID is required")
	}
	if len(cfg.AuthorizedUsers) == 0 {
		return nil, fmt.Errorf("telegram authorized_users is required — at least one user ID must be configured")
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = defaultTimeout
	}

	ta := &TelegramApprover{
		cfg:     cfg,
		client:  &http.Client{Timeout: time.Duration(pollTimeout+5) * time.Second},
		baseURL: fmt.Sprintf("https://api.telegram.org/bot%s", cfg.BotToken),
		pending: make(map[string]chan Result),
		stopCh:  make(chan struct{}),
	}

	go ta.pollUpdates()
	return ta, nil
}

// Close stops the polling goroutine.
func (t *TelegramApprover) Close() {
	t.stopOnce.Do(func() { close(t.stopCh) })
}

// --- Public API ---

// RequestApproval sends a Telegram message with inline approve/reject buttons
// and blocks until the user responds or the context/timeout expires.
func (t *TelegramApprover) RequestApproval(ctx context.Context, req Request) (Result, error) {
	if req.ID == "" {
		req.ID = uuid.New().String()
	}

	resultCh := make(chan Result, 1)
	t.mu.Lock()
	t.pending[req.ID] = resultCh
	t.mu.Unlock()
	defer t.removePending(req.ID)

	msgID, err := t.sendApprovalMessage(ctx, req)
	if err != nil {
		return Result{}, fmt.Errorf("sending approval message: %w", t.sanitize(err))
	}

	timeout := time.Until(req.ExpiresAt)
	if timeout <= 0 {
		timeout = time.Duration(t.cfg.TimeoutSeconds) * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case result := <-resultCh:
		t.editMessageResult(ctx, msgID, req, result)
		return result, nil
	case <-timer.C:
		t.editMessageTimeout(ctx, msgID, req)
		return Result{Approved: false, Reason: "approval timed out"}, nil
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

// NotifyBroadcast sends a follow-up message with the broadcast result.
func (t *TelegramApprover) NotifyBroadcast(ctx context.Context, txid string, success bool) error {
	var text string
	if success {
		text = fmt.Sprintf(
			"✅ *Transaction Broadcast Successful*\n\n"+
				"🔗 *TxID:*\n`%s`\n\n"+
				"_View on TronScan: tronscan.org/#/transaction/%s_",
			txid, txid,
		)
	} else {
		text = fmt.Sprintf(
			"❌ *Transaction Broadcast Failed*\n\n"+
				"🔗 *TxID:* `%s`\n\n"+
				"_The transaction was signed but failed to broadcast._",
			txid,
		)
	}
	_, err := t.sendMessage(ctx, text, "")
	return err
}

// --- Message formatting ---

// formatApprovalMessage builds a human-readable approval request message.
func formatApprovalMessage(req Request) string {
	var b strings.Builder

	// Header
	if req.IsOverride {
		fmt.Fprint(&b, "⚠️ *Above-Limit Transaction Request*\n")
	} else {
		fmt.Fprint(&b, "🔔 *Transaction Approval Request*\n")
	}
	fmt.Fprint(&b, "━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// Wallet & type
	writeField(&b, "💼", "Wallet", req.WalletName)
	writeField(&b, "📋", "Type", req.ContractType)
	fmt.Fprint(&b, "\n")

	// Addresses
	writeContractField(&b, "📤", "From", req.ContractData, "owner_address")
	writeContractField(&b, "📥", "To", req.ContractData, "to_address")
	writeContractField(&b, "📝", "Contract", req.ContractData, "contract_address")
	fmt.Fprint(&b, "\n")

	// Amount
	writeContractField(&b, "💰", "Amount", req.ContractData, "amount")

	// Spend context
	if req.SpendContext != "" {
		fmt.Fprintf(&b, "📊 *Spend:* %s\n", req.SpendContext)
	}

	// Agent reason
	if req.Reason != "" {
		fmt.Fprintf(&b, "\n🤖 *Reason:* _%s_\n", req.Reason)
	}

	// Summary
	if req.HumanSummary != "" {
		fmt.Fprintf(&b, "💬 _%s_\n", req.HumanSummary)
	}

	// Expiry
	if remaining := time.Until(req.ExpiresAt).Round(time.Second); remaining > 0 {
		fmt.Fprintf(&b, "\n⏰ *Expires in %s* (%s)\n", remaining, req.ExpiresAt.Format("15:04:05 UTC"))
	} else {
		fmt.Fprintf(&b, "\n⏰ Expires: %s\n", req.ExpiresAt.Format("15:04:05 UTC"))
	}

	// Footer
	if req.IsOverride {
		fmt.Fprint(&b, "\n⚠️ _This exceeds your configured limits. One-time approval only._")
	} else {
		fmt.Fprint(&b, "\n⚠️ _This transaction will be signed and broadcast upon approval._")
	}

	return b.String()
}

func writeField(b *strings.Builder, emoji, label, value string) {
	if value != "" {
		fmt.Fprintf(b, "%s *%s:* `%s`\n", emoji, label, value)
	}
}

func writeContractField(b *strings.Builder, emoji, label string, data map[string]any, key string) {
	if v, ok := data[key].(string); ok && v != "" {
		fmt.Fprintf(b, "%s *%s:*\n`%s`\n", emoji, label, v)
	}
}

// --- Telegram API methods ---

func (t *TelegramApprover) sendApprovalMessage(ctx context.Context, req Request) (int64, error) {
	text := formatApprovalMessage(req)
	keyboard := buildInlineKeyboard(req.ID)
	return t.sendMessage(ctx, text, keyboard)
}

func buildInlineKeyboard(approvalID string) string {
	return fmt.Sprintf(
		`{"inline_keyboard":[[{"text":"✅ Approve","callback_data":"approve:%s"},{"text":"❌ Reject","callback_data":"reject:%s"}]]}`,
		approvalID, approvalID,
	)
}

func (t *TelegramApprover) sendMessage(ctx context.Context, text, replyMarkup string) (int64, error) {
	params := url.Values{
		"chat_id":    {t.chatIDStr()},
		"text":       {text},
		"parse_mode": {"Markdown"},
	}
	if replyMarkup != "" {
		params.Set("reply_markup", replyMarkup)
	}

	resp, err := t.doPost(ctx, methodSendMessage, params)
	if err != nil {
		return 0, err
	}
	msgID, _ := resp["message_id"].(float64)
	return int64(msgID), nil
}

func (t *TelegramApprover) editMessageResult(ctx context.Context, msgID int64, req Request, result Result) {
	status := "✅ *APPROVED*"
	if !result.Approved {
		status = "❌ *REJECTED*"
	}
	text := fmt.Sprintf("%s\n\n%s by %s at %s",
		formatApprovalMessage(req),
		status,
		result.ApprovedBy,
		result.Timestamp.Format("15:04:05 UTC"),
	)
	t.editMessage(ctx, msgID, text)
}

func (t *TelegramApprover) editMessageTimeout(ctx context.Context, msgID int64, req Request) {
	text := fmt.Sprintf("%s\n\n⏰ *EXPIRED* — approval timed out", formatApprovalMessage(req))
	t.editMessage(ctx, msgID, text)
}

func (t *TelegramApprover) editMessage(ctx context.Context, msgID int64, text string) {
	params := url.Values{
		"chat_id":    {t.chatIDStr()},
		"message_id": {strconv.FormatInt(msgID, 10)},
		"text":       {text},
		"parse_mode": {"Markdown"},
	}
	if _, err := t.doPost(ctx, methodEditMessageText, params); err != nil {
		log.Printf("telegram: failed to edit message %d: %v", msgID, t.sanitize(err))
	}
}

func (t *TelegramApprover) answerCallbackQuery(cbQuery map[string]any, text string) {
	cbID, _ := cbQuery["id"].(string)
	if cbID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	params := url.Values{
		"callback_query_id": {cbID},
		"text":              {text},
	}
	if _, err := t.doPost(ctx, methodAnswerCallbackQuery, params); err != nil {
		log.Printf("telegram: failed to answer callback: %v", t.sanitize(err))
	}
}

// --- Polling ---

func (t *TelegramApprover) pollUpdates() {
	var offset int64
	for {
		select {
		case <-t.stopCh:
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(pollTimeout+5)*time.Second)
		params := url.Values{
			"timeout":         {strconv.Itoa(pollTimeout)},
			"allowed_updates": {`["callback_query"]`},
		}
		if offset > 0 {
			params.Set("offset", strconv.FormatInt(offset, 10))
		}

		body, err := t.doGet(ctx, methodGetUpdates, params)
		cancel()
		if err != nil {
			select {
			case <-t.stopCh:
				return
			default:
				log.Printf("telegram: poll error: %v", t.sanitize(err))
				time.Sleep(pollRetryDelay)
			}
			continue
		}

		updates, ok := body["result"].([]any)
		if !ok {
			continue
		}

		for _, u := range updates {
			update, ok := u.(map[string]any)
			if !ok {
				continue
			}
			if updateID, ok := update["update_id"].(float64); ok {
				offset = int64(updateID) + 1
			}
			t.handleCallbackQuery(update)
		}
	}
}

// --- Callback handling ---

func (t *TelegramApprover) handleCallbackQuery(update map[string]any) {
	cbQuery, ok := update["callback_query"].(map[string]any)
	if !ok {
		return
	}

	action, approvalID, ok := parseCallbackData(cbQuery)
	if !ok {
		return
	}

	if !t.isFromConfiguredChat(cbQuery) {
		return
	}

	userID, username := extractUser(cbQuery)
	if !t.isAuthorized(userID) {
		t.answerCallbackQuery(cbQuery, "⛔ You are not authorized to approve transactions.")
		return
	}

	t.mu.Lock()
	resultCh, exists := t.pending[approvalID]
	t.mu.Unlock()

	if !exists {
		t.answerCallbackQuery(cbQuery, "⏰ This approval request has expired or was already handled.")
		return
	}

	result := Result{
		Approved:   action == "approve",
		ApprovedBy: fmt.Sprintf("%s (ID:%d)", username, userID),
		Timestamp:  time.Now().UTC(),
	}
	if !result.Approved {
		result.Reason = "rejected by user"
	}

	select {
	case resultCh <- result:
	default:
	}

	if result.Approved {
		t.answerCallbackQuery(cbQuery, "✅ Transaction approved!")
	} else {
		t.answerCallbackQuery(cbQuery, "❌ Transaction rejected.")
	}
}

func parseCallbackData(cbQuery map[string]any) (action, approvalID string, ok bool) {
	data, _ := cbQuery["data"].(string)
	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (t *TelegramApprover) isFromConfiguredChat(cbQuery map[string]any) bool {
	msg, ok := cbQuery["message"].(map[string]any)
	if !ok {
		return false
	}
	chat, ok := msg["chat"].(map[string]any)
	if !ok {
		return false
	}
	chatID, ok := chat["id"].(float64)
	if !ok {
		return false
	}
	return int64(chatID) == t.cfg.ChatID
}

func extractUser(cbQuery map[string]any) (int64, string) {
	from, _ := cbQuery["from"].(map[string]any)
	var userID int64
	if id, ok := from["id"].(float64); ok {
		userID = int64(id)
	}
	username := "unknown"
	if u, ok := from["username"].(string); ok {
		username = "@" + u
	} else if fn, ok := from["first_name"].(string); ok {
		username = fn
	}
	return userID, username
}

func (t *TelegramApprover) isAuthorized(userID int64) bool {
	for _, id := range t.cfg.AuthorizedUsers {
		if id == userID {
			return true
		}
	}
	return false
}

// --- HTTP helpers ---

func (t *TelegramApprover) doPost(ctx context.Context, method string, params url.Values) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+method, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return t.doRequest(req)
}

func (t *TelegramApprover) doGet(ctx context.Context, method string, params url.Values) (map[string]any, error) {
	u := t.baseURL + method
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	return t.doRequest(req)
}

func (t *TelegramApprover) doRequest(req *http.Request) (map[string]any, error) {
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if ok, _ := result["ok"].(bool); !ok {
		desc, _ := result["description"].(string)
		return nil, fmt.Errorf("telegram API error: %s", desc)
	}

	if r, ok := result["result"].(map[string]any); ok {
		return r, nil
	}
	return result, nil
}

// --- Utilities ---

func (t *TelegramApprover) chatIDStr() string {
	return strconv.FormatInt(t.cfg.ChatID, 10)
}

func (t *TelegramApprover) removePending(id string) {
	t.mu.Lock()
	delete(t.pending, id)
	t.mu.Unlock()
}

func (t *TelegramApprover) sanitize(err error) error {
	return sanitizeError(err, t.cfg.BotToken)
}

// sanitizeError removes the bot token from error messages to prevent log leakage.
func sanitizeError(err error, token string) error {
	if err == nil || token == "" {
		return err
	}
	return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), token, "***"))
}
