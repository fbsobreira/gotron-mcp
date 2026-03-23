package retry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDo_SuccessFirstAttempt(t *testing.T) {
	calls := 0
	result, err := Do(func() (string, error) {
		calls++
		return "ok", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.Equal(t, 1, calls)
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
	require.NoError(t, err)
	assert.Equal(t, 42, result)
	assert.Equal(t, 3, calls)
}

func TestDo_ExhaustedRetries(t *testing.T) {
	calls := 0
	_, err := Do(func() (string, error) {
		calls++
		return "", status.Error(codes.Unavailable, "always down")
	})
	require.Error(t, err, "expected error after exhausted retries")
	assert.Equal(t, maxRetries, calls)
}

func TestDo_NonRetryableError(t *testing.T) {
	calls := 0
	_, err := Do(func() (string, error) {
		calls++
		return "", status.Error(codes.InvalidArgument, "bad input")
	})
	require.Error(t, err)
	assert.Equal(t, 1, calls, "non-retryable error should not retry")
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
			assert.Equal(t, tt.want, got, "isRetryable(%v)", tt.err)
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
	assert.GreaterOrEqual(t, elapsed, 500*time.Millisecond, "retries completed too fast (expected backoff delays)")
	assert.LessOrEqual(t, elapsed, 10*time.Second, "retries took too long")
}

func TestDoCtx_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	calls := 0
	_, err := DoCtx(ctx, func(_ context.Context) (string, error) {
		calls++
		return "", status.Error(codes.Unavailable, "down")
	})
	assert.Equal(t, context.Canceled, err)
	assert.Equal(t, 1, calls, "expected 1 call before cancel detected")
}

func TestDoCtx_CancelDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	calls := 0
	_, err := DoCtx(ctx, func(_ context.Context) (string, error) {
		calls++
		return "", status.Error(codes.Unavailable, "down")
	})
	elapsed := time.Since(start)

	require.Error(t, err)
	// Should abort during first backoff sleep (~500ms), not complete all retries (~2s)
	assert.LessOrEqual(t, elapsed, 1*time.Second, "should have aborted during backoff")
}

// mockFailoverer implements Failoverer for testing.
type mockFailoverer struct {
	called     int
	willSwitch bool
}

func (m *mockFailoverer) Failover() bool {
	m.called++
	return m.willSwitch
}

func TestDoWithFailover_SuccessNoFailover(t *testing.T) {
	f := &mockFailoverer{}
	result, err := DoWithFailover(context.Background(), f, func(_ context.Context) (string, error) {
		return "ok", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.Equal(t, 0, f.called, "failover should not be called on success")
}

func TestDoWithFailover_FailoverAndRecover(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping failover timing test in short mode")
	}

	f := &mockFailoverer{willSwitch: true}
	calls := 0
	result, err := DoWithFailover(context.Background(), f, func(_ context.Context) (int, error) {
		calls++
		// Fail for first round (maxRetries), succeed on first call of second round
		if calls <= maxRetries {
			return 0, status.Error(codes.Unavailable, "node down")
		}
		return 42, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 42, result)
	assert.Equal(t, 1, f.called, "expected 1 failover call")
	assert.Equal(t, maxRetries+1, calls, "expected %d total calls", maxRetries+1)
}

func TestDoWithFailover_NoFallbackAvailable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping failover timing test in short mode")
	}

	f := &mockFailoverer{willSwitch: false}
	calls := 0
	_, err := DoWithFailover(context.Background(), f, func(_ context.Context) (string, error) {
		calls++
		return "", status.Error(codes.Unavailable, "down")
	})
	require.Error(t, err)
	assert.Equal(t, 1, f.called, "expected 1 failover attempt")
	assert.Equal(t, maxRetries, calls, "expected %d calls (no second round)", maxRetries)
}

func TestDoWithFailover_NonRetryableSkipsFailover(t *testing.T) {
	f := &mockFailoverer{willSwitch: true}
	_, err := DoWithFailover(context.Background(), f, func(_ context.Context) (string, error) {
		return "", status.Error(codes.InvalidArgument, "bad input")
	})
	require.Error(t, err)
	assert.Equal(t, 0, f.called, "failover should not be called for non-retryable errors")
}
