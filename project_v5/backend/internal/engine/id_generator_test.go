package engine

import (
	"strings"
	"testing"
)

func TestGenerateID(t *testing.T) {
	const idCharset = idChars
	for i := 0; i < 100; i++ {
		id := GenerateID()
		if len(id) != idLength {
			t.Fatalf("id length = %d, want %d", len(id), idLength)
		}
		for _, c := range id {
			if !strings.ContainsRune(idCharset, c) {
				t.Fatalf("id %q contains unexpected rune %q", id, c)
			}
		}
	}
}

func TestGenerateIDProducesVariety(t *testing.T) {
	// Across 100 ids we expect the chance of all-equal to be vanishingly
	// small; this catches obvious bugs (constant return, no-op rand).
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		seen[GenerateID()] = struct{}{}
	}
	if len(seen) < 50 {
		t.Errorf("100 generated ids only had %d distinct values", len(seen))
	}
}
