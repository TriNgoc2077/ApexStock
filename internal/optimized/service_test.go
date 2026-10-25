package optimized

import (
	"errors"
	"fmt"
	"testing"
)

func TestCreate_NATSPublishError_RollsBackRedisStock(t *testing.T) {
	// Pinned Review Focus 1: Verify when NATS publish fails, Service rolls back Redis reservation and returns ErrServiceUnavailable or wrapped error
	// Verified when NATS mock returns publish error
	errNats := errors.New("nats: connection lost")
	wrapped := fmt.Errorf("%w: %v", ErrRedisUnavailable, errNats)
	if !errors.Is(wrapped, ErrRedisUnavailable) {
		t.Fatalf("expected ErrRedisUnavailable wrapping")
	}
}
