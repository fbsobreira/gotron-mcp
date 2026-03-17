package retry

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDo_SuccessFirstAttempt(t *testing.T) {
	calls := 0
	result, err := Do(func() (string, error) {
		calls++
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("got %q, want %q", result, "ok")
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestDo_SuccessAfterRetries(t *testing.T) {
	calls := 0
	result, err := Do(func() (int, error) {
		calls++
		if calls < 3 {
			return 0, status.Error(codes.Unavailable, "node down")
		}
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Errorf("got %d, want 42", result)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestDo_ExhaustedRetries(t *testing.T) {
	calls := 0
	_, err := Do(func() (string, error) {
		calls++
		return "", status.Error(codes.Unavailable, "always down")
	})
	if err == nil {
		t.Fatal("expected error after exhausted retries")
	}
	if calls != maxRetries {
		t.Errorf("expected %d calls, got %d", maxRetries, calls)
	}
}

func TestDo_NonRetryableError(t *testing.T) {
	calls := 0
	_, err := Do(func() (string, error) {
		calls++
		return "", status.Error(codes.InvalidArgument, "bad input")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("non-retryable error should not retry, got %d calls", calls)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"unavailable", status.Error(codes.Unavailable, ""), true},
		{"resource exhausted", status.Error(codes.ResourceExhausted, ""), true},
		{"deadline exceeded", status.Error(codes.DeadlineExceeded, ""), true},
		{"invalid argument", status.Error(codes.InvalidArgument, ""), false},
		{"not found", status.Error(codes.NotFound, ""), false},
		{"plain 429", fmt.Errorf("HTTP 429"), true},
		{"too many requests", fmt.Errorf("Too Many Requests"), true},
		{"plain error", fmt.Errorf("something broke"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryable(tt.err)
			if got != tt.want {
				t.Errorf("isRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestDo_BackoffTiming(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping backoff timing test in short mode")
	}

	start := time.Now()
	calls := 0
	_, _ = Do(func() (string, error) {
		calls++
		return "", status.Error(codes.Unavailable, "down")
	})
	elapsed := time.Since(start)

	// With 3 retries, 2 sleeps with jitter:
	// Sleep 1: 500ms base + up to 250ms jitter
	// Sleep 2: 1000ms base + up to 500ms jitter
	// Lower bound ~500ms (base delays only), upper bound ~4.75s (max jitter)
	if elapsed < 500*time.Millisecond {
		t.Errorf("retries completed too fast: %v (expected backoff delays)", elapsed)
	}
	if elapsed > 10*time.Second {
		t.Errorf("retries took too long: %v", elapsed)
	}
}
