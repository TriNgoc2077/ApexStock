package optimized

import (
	"regexp"
	"testing"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewID_FormatAndEntropy(t *testing.T) {
	const count = 10000
	seen := make(map[string]struct{}, count)

	for i := 0; i < count; i++ {
		id := newID()
		if !uuidRegex.MatchString(id) {
			t.Fatalf("generated ID %q does not match RFC 4122 v4 UUID pattern", id)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("UUID collision detected at iteration %d: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}
