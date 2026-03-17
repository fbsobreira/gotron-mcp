package retry

import (
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
	var lastErr error
	var zero T

	for attempt := range maxRetries {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		if !isRetryable(err) {
			return zero, err
		}

		lastErr = err
		if attempt < maxRetries-1 {
			delay := min(baseDelay*time.Duration(1<<uint(attempt)), maxDelay)
			jitter := time.Duration(rand.Int64N(int64(delay) / 2))
			time.Sleep(delay + jitter)
		}
	}

	return zero, lastErr
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
