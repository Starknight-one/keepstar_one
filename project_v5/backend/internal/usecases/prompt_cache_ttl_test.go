package usecases

import (
	"context"
	"testing"
	"time"

	"keepstar_v5/internal/domain"
)

// countingFieldDefPort wraps samplingFieldDefPort (prompt_cache_test.go)
// counting the <fields>-block sample reads — the observable proxy for an
// Agent2 prompt rebuild vs a cache hit.
type countingFieldDefPort struct {
	samplingFieldDefPort
	samples int
}

func (c *countingFieldDefPort) SampleFieldValues(ctx context.Context, tenant string, et domain.EntityType, n int) (map[string][]interface{}, error) {
	c.samples++
	return c.samplingFieldDefPort.SampleFieldValues(ctx, tenant, et, n)
}

// §6.1 TTL safety net on the Agent2 prompt cache: an entry older than the
// TTL is a miss (rebuild); a fresh entry is a hit. Without this, a missed
// best-effort admin→v5 invalidation would serve a stale prompt forever.
func TestPromptCache_TTLExpiry(t *testing.T) {
	fd := &countingFieldDefPort{}
	pc := NewPromptCache(fd, noopPresetPort{}, &flagTenantCatalog{}, "product")
	pc.ttl = time.Millisecond

	if _, err := pc.GetOrBuild(context.Background(), "acme", 3); err != nil {
		t.Fatalf("GetOrBuild: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := pc.GetOrBuild(context.Background(), "acme", 3); err != nil {
		t.Fatalf("GetOrBuild (after TTL): %v", err)
	}
	if fd.samples != 2 {
		t.Errorf("field samples = %d, want 2 (TTL-expired entry must rebuild)", fd.samples)
	}

	// Fresh entries stay hits: bump TTL up, read again, no rebuild.
	pc.ttl = time.Hour
	if _, err := pc.GetOrBuild(context.Background(), "acme", 3); err != nil {
		t.Fatalf("GetOrBuild (fresh): %v", err)
	}
	if fd.samples != 2 {
		t.Errorf("field samples = %d, want 2 (fresh entry must be a hit)", fd.samples)
	}
}
