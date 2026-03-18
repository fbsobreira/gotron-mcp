package retry

import (
	"context"
	"math/rand/v2"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	maxRetries = 3
	baseDelay  = 500 * time.Millisecond
	maxDelay   = 5 * time.Second
)

// Do retries fn with exponential backoff for transient gRPC errors.
func Do[T any](fn func() (T, error)) (T, error) {
	return DoCtx(context.Background(), func(_ context.Context) (T, error) {
		return fn()
	})
}

// DoCtx retries fn with exponential backoff, respecting context cancellation.
// Backoff sleeps abort immediately when ctx is cancelled.
func DoCtx[T any](ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	var lastErr error
	var zero T

	for attempt := range maxRetries {
		result, err := fn(ctx)
		if err == nil {
			return result, nil
		}

		if ctx.Err() != nil {
			return zero, ctx.Err()
		}

		if !isRetryable(err) {
			return zero, err
		}

		lastErr = err
		if attempt < maxRetries-1 {
			delay := min(baseDelay*time.Duration(1<<uint(attempt)), maxDelay)
			jitter := time.Duration(rand.Int64N(int64(delay) / 2))
			select {
			case <-time.After(delay + jitter):
			case <-ctx.Done():
				return zero, ctx.Err()
			}
		}
	}

	return zero, lastErr
}

// Failoverer is implemented by nodepool.Pool to trigger failover on exhausted retries.
type Failoverer interface {
	Failover() bool
}

// DoWithFailover retries fn, and if all retries on the current node are exhausted
// with retryable errors, triggers failover and retries again on the new node.
func DoWithFailover[T any](ctx context.Context, f Failoverer, fn func(context.Context) (T, error)) (T, error) {
	result, err := DoCtx(ctx, fn)
	if err == nil {
		return result, nil
	}

	// Only failover on retryable errors (non-retryable means the request itself is bad)
	if !isRetryable(err) || ctx.Err() != nil {
		return result, err
	}

	// Try failover — if successful, retry on the new node
	if f.Failover() {
		return DoCtx(ctx, fn)
	}

	return result, err
}

func isRetryable(err error) bool {
	st, ok := status.FromError(err)
	if ok {
		switch st.Code() {
		case codes.Unavailable, codes.ResourceExhausted, codes.DeadlineExceeded:
			return true
		}
	}
	// TronGrid returns rate limit errors as text sometimes
	msg := err.Error()
	return strings.Contains(msg, "429") || strings.Contains(msg, "Too Many Requests")
}
