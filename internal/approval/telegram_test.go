package approval

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMockTelegramServer creates an httptest server that responds to Telegram Bot API calls.
func newMockTelegramServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case containsPath(r.URL.Path, "/sendMessage"):
			_, _ = fmt.Fprint(w, `{"ok":true,"result":{"message_id":1}}`)
		case containsPath(r.URL.Path, "/getUpdates"):
			_, _ = fmt.Fprint(w, `{"ok":true,"result":[]}`)
		case containsPath(r.URL.Path, "/answerCallbackQuery"):
			_, _ = fmt.Fprint(w, `{"ok":true,"result":true}`)
		case containsPath(r.URL.Path, "/editMessageText"):
			_, _ = fmt.Fprint(w, `{"ok":true,"result":{}}`)
		default:
			_, _ = fmt.Fprint(w, `{"ok":true,"result":{}}`)
		}
	}))
}

func containsPath(path, method string) bool {
	return len(path) >= len(method) && path[len(path)-len(method):] == method
}

// newTestTelegramApprover creates a TelegramApprover pointing at the mock server.
func newTestTelegramApprover(t *testing.T, srv *httptest.Server) *TelegramApprover {
	t.Helper()
	return &TelegramApprover{
		cfg:     TelegramConfig{BotToken: "test-token", ChatID: 123, AuthorizedUsers: []int64{111}, TimeoutSeconds: 1},
		client:  &http.Client{Timeout: 5 * time.Second},
		baseURL: srv.URL,
		pending: make(map[string]chan Result),
		stopCh:  make(chan struct{}),
	}
}

func TestSanitizeError(t *testing.T) {
	t.Run("NilError", func(t *testing.T) {
		assert.Nil(t, sanitizeError(nil, "secret-token"))
	})

	t.Run("EmptyToken", func(t *testing.T) {
		err := fmt.Errorf("some error")
		result := sanitizeError(err, "")
		assert.Equal(t, "some error", result.Error())
	})

	t.Run("TokenPresent", func(t *testing.T) {
		err := fmt.Errorf("failed to call https://api.telegram.org/botsecret-token/sendMessage")
		result := sanitizeError(err, "secret-token")
		assert.NotContains(t, result.Error(), "secret-token")
		assert.Contains(t, result.Error(), "***")
	})
}

func TestTelegramApprover_Close(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()
	ta := newTestTelegramApprover(t, srv)

	// Close should not panic
	assert.NotPanics(t, func() {
		ta.Close()
	})

	// Double close should not panic
	assert.NotPanics(t, func() {
		ta.Close()
	})
}

func TestRequestApproval_Timeout(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()
	ta := newTestTelegramApprover(t, srv)
	defer ta.Close()

	ctx := context.Background()
	req := Request{
		WalletName:   "test-wallet",
		ContractType: "TransferContract",
		ContractData: map[string]any{"amount": "1 TRX"},
		ExpiresAt:    time.Now().Add(100 * time.Millisecond), // very short timeout
	}

	result, err := ta.RequestApproval(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.Approved)
	assert.Contains(t, result.Reason, "timed out")
}

func TestHandleCallbackQuery_Approve(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()
	ta := newTestTelegramApprover(t, srv)
	defer ta.Close()

	approvalID := "test-approve-id"
	resultCh := make(chan Result, 1)
	ta.mu.Lock()
	ta.pending[approvalID] = resultCh
	ta.mu.Unlock()

	update := map[string]any{
		"callback_query": map[string]any{
			"id":   "cb-1",
			"data": "approve:" + approvalID,
			"from": map[string]any{
				"id":       float64(111),
				"username": "testuser",
			},
			"message": map[string]any{
				"chat": map[string]any{
					"id": float64(123),
				},
			},
		},
	}

	ta.handleCallbackQuery(update)

	select {
	case result := <-resultCh:
		assert.True(t, result.Approved)
		assert.Contains(t, result.ApprovedBy, "@testuser")
		assert.Contains(t, result.ApprovedBy, "111")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for approval result")
	}
}

func TestHandleCallbackQuery_Reject(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()
	ta := newTestTelegramApprover(t, srv)
	defer ta.Close()

	approvalID := "test-reject-id"
	resultCh := make(chan Result, 1)
	ta.mu.Lock()
	ta.pending[approvalID] = resultCh
	ta.mu.Unlock()

	update := map[string]any{
		"callback_query": map[string]any{
			"id":   "cb-2",
			"data": "reject:" + approvalID,
			"from": map[string]any{
				"id":       float64(111),
				"username": "testuser",
			},
			"message": map[string]any{
				"chat": map[string]any{
					"id": float64(123),
				},
			},
		},
	}

	ta.handleCallbackQuery(update)

	select {
	case result := <-resultCh:
		assert.False(t, result.Approved)
		assert.Equal(t, "rejected by user", result.Reason)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for rejection result")
	}
}

func TestHandleCallbackQuery_WrongChat(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()
	ta := newTestTelegramApprover(t, srv)
	defer ta.Close()

	approvalID := "test-wrong-chat"
	resultCh := make(chan Result, 1)
	ta.mu.Lock()
	ta.pending[approvalID] = resultCh
	ta.mu.Unlock()

	update := map[string]any{
		"callback_query": map[string]any{
			"id":   "cb-3",
			"data": "approve:" + approvalID,
			"from": map[string]any{
				"id":       float64(111),
				"username": "testuser",
			},
			"message": map[string]any{
				"chat": map[string]any{
					"id": float64(999), // wrong chat ID
				},
			},
		},
	}

	ta.handleCallbackQuery(update)

	// Result channel should remain empty — callback was ignored
	select {
	case <-resultCh:
		t.Fatal("should not have received a result for wrong chat")
	case <-time.After(100 * time.Millisecond):
		// expected: no result
	}
}

func TestHandleCallbackQuery_Unauthorized(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()
	ta := newTestTelegramApprover(t, srv)
	defer ta.Close()

	approvalID := "test-unauthorized"
	resultCh := make(chan Result, 1)
	ta.mu.Lock()
	ta.pending[approvalID] = resultCh
	ta.mu.Unlock()

	update := map[string]any{
		"callback_query": map[string]any{
			"id":   "cb-4",
			"data": "approve:" + approvalID,
			"from": map[string]any{
				"id":       float64(999), // not in AuthorizedUsers
				"username": "hacker",
			},
			"message": map[string]any{
				"chat": map[string]any{
					"id": float64(123),
				},
			},
		},
	}

	ta.handleCallbackQuery(update)

	// Result channel should remain empty — user is not authorized
	select {
	case <-resultCh:
		t.Fatal("should not have received a result for unauthorized user")
	case <-time.After(100 * time.Millisecond):
		// expected: no result
	}
}

func TestHandleCallbackQuery_Expired(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()
	ta := newTestTelegramApprover(t, srv)
	defer ta.Close()

	// Do NOT add any pending entry — simulates expired/already handled

	update := map[string]any{
		"callback_query": map[string]any{
			"id":   "cb-5",
			"data": "approve:non-existing-id",
			"from": map[string]any{
				"id":       float64(111),
				"username": "testuser",
			},
			"message": map[string]any{
				"chat": map[string]any{
					"id": float64(123),
				},
			},
		},
	}

	// Should not panic; just answers with "expired" toast
	assert.NotPanics(t, func() {
		ta.handleCallbackQuery(update)
	})
}

func TestNotifyBroadcast_Success(t *testing.T) {
	var receivedText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := r.ParseForm(); err == nil {
			if text := r.FormValue("text"); text != "" {
				receivedText = text
			}
		}
		_, _ = fmt.Fprint(w, `{"ok":true,"result":{"message_id":2}}`)
	}))
	defer srv.Close()

	ta := newTestTelegramApprover(t, srv)
	defer ta.Close()

	err := ta.NotifyBroadcast(context.Background(), "abc123txid", true)
	require.NoError(t, err)
	assert.Contains(t, receivedText, "Successful")
	assert.Contains(t, receivedText, "abc123txid")
}

func TestNotifyBroadcast_Failure(t *testing.T) {
	var receivedText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := r.ParseForm(); err == nil {
			if text := r.FormValue("text"); text != "" {
				receivedText = text
			}
		}
		_, _ = fmt.Fprint(w, `{"ok":true,"result":{"message_id":3}}`)
	}))
	defer srv.Close()

	ta := newTestTelegramApprover(t, srv)
	defer ta.Close()

	err := ta.NotifyBroadcast(context.Background(), "fail-txid", false)
	require.NoError(t, err)
	assert.Contains(t, receivedText, "Failed")
	assert.Contains(t, receivedText, "fail-txid")
}

func TestEditMessageResult_Approved(t *testing.T) {
	var editedText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := r.ParseForm(); err == nil {
			if text := r.FormValue("text"); text != "" {
				editedText = text
			}
		}
		_, _ = fmt.Fprint(w, `{"ok":true,"result":{}}`)
	}))
	defer srv.Close()

	ta := newTestTelegramApprover(t, srv)
	defer ta.Close()

	req := Request{
		WalletName:   "test",
		ContractType: "TransferContract",
		ContractData: map[string]any{},
		ExpiresAt:    time.Now().Add(5 * time.Minute),
	}
	result := Result{Approved: true, ApprovedBy: "@admin", Timestamp: time.Now().UTC()}
	ta.editMessageResult(context.Background(), 1, req, result)
	assert.Contains(t, editedText, "APPROVED")
	assert.Contains(t, editedText, "@admin")
}

func TestEditMessageResult_Rejected(t *testing.T) {
	var editedText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := r.ParseForm(); err == nil {
			if text := r.FormValue("text"); text != "" {
				editedText = text
			}
		}
		_, _ = fmt.Fprint(w, `{"ok":true,"result":{}}`)
	}))
	defer srv.Close()

	ta := newTestTelegramApprover(t, srv)
	defer ta.Close()

	req := Request{ContractData: map[string]any{}, ExpiresAt: time.Now().Add(5 * time.Minute)}
	result := Result{Approved: false, ApprovedBy: "@user", Timestamp: time.Now().UTC()}
	ta.editMessageResult(context.Background(), 1, req, result)
	assert.Contains(t, editedText, "REJECTED")
}

func TestEditMessageTimeout(t *testing.T) {
	var editedText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := r.ParseForm(); err == nil {
			if text := r.FormValue("text"); text != "" {
				editedText = text
			}
		}
		_, _ = fmt.Fprint(w, `{"ok":true,"result":{}}`)
	}))
	defer srv.Close()

	ta := newTestTelegramApprover(t, srv)
	defer ta.Close()

	req := Request{ContractData: map[string]any{}, ExpiresAt: time.Now().Add(5 * time.Minute)}
	ta.editMessageTimeout(context.Background(), 1, req)
	assert.Contains(t, editedText, "EXPIRED")
}

func TestDoGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assert.Equal(t, http.MethodGet, r.Method)
		_, _ = fmt.Fprint(w, `{"ok":true,"result":[]}`)
	}))
	defer srv.Close()

	ta := newTestTelegramApprover(t, srv)
	defer ta.Close()

	result, err := ta.doGet(context.Background(), "/getUpdates", nil)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestDoRequest_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"ok":false,"description":"Unauthorized"}`)
	}))
	defer srv.Close()

	ta := newTestTelegramApprover(t, srv)
	defer ta.Close()

	_, err := ta.doPost(context.Background(), "/sendMessage", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Unauthorized")
}

func TestDoRequest_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "not json")
	}))
	defer srv.Close()

	ta := newTestTelegramApprover(t, srv)
	defer ta.Close()

	_, err := ta.doPost(context.Background(), "/sendMessage", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing response")
}

func TestRequestApproval_FullFlow(t *testing.T) {
	t.Skip("Integration test — run manually. Flaky in CI due to polling goroutine + httptest server lifecycle.")
	// Mock server that returns a callback on the second getUpdates call
	callCount := 0
	var approvalID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case containsPath(r.URL.Path, "/sendMessage"):
			_, _ = fmt.Fprint(w, `{"ok":true,"result":{"message_id":42}}`)
		case containsPath(r.URL.Path, "/getUpdates"):
			callCount++
			if callCount >= 2 && approvalID != "" {
				// Return an approve callback
				_, _ = fmt.Fprintf(w, `{"ok":true,"result":[{"update_id":1,"callback_query":{"id":"cb1","data":"approve:%s","from":{"id":111,"username":"admin"},"message":{"chat":{"id":123}}}}]}`, approvalID)
			} else {
				_, _ = fmt.Fprint(w, `{"ok":true,"result":[]}`)
			}
		case containsPath(r.URL.Path, "/answerCallbackQuery"):
			_, _ = fmt.Fprint(w, `{"ok":true,"result":true}`)
		case containsPath(r.URL.Path, "/editMessageText"):
			_, _ = fmt.Fprint(w, `{"ok":true,"result":{}}`)
		default:
			_, _ = fmt.Fprint(w, `{"ok":true,"result":{}}`)
		}
	}))
	ta := &TelegramApprover{
		cfg:     TelegramConfig{BotToken: "test", ChatID: 123, AuthorizedUsers: []int64{111}, TimeoutSeconds: 10},
		client:  &http.Client{Timeout: 2 * time.Second},
		baseURL: srv.URL,
		pending: make(map[string]chan Result),
		stopCh:  make(chan struct{}),
	}

	// Cleanup: close approver first (stops poller), then server
	t.Cleanup(func() {
		ta.Close()
		time.Sleep(100 * time.Millisecond)
		srv.Close()
	})

	// Start polling
	go ta.pollUpdates()

	// Set the approval ID after a short delay (simulates the send happening first)
	go func() {
		time.Sleep(200 * time.Millisecond)
		ta.mu.Lock()
		for id := range ta.pending {
			approvalID = id
		}
		ta.mu.Unlock()
	}()

	req := Request{
		ContractType: "TransferContract",
		ContractData: map[string]any{},
		ExpiresAt:    time.Now().Add(10 * time.Second),
	}
	result, err := ta.RequestApproval(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.Approved)
	assert.Contains(t, result.ApprovedBy, "@admin")
}
