package web

import (
	"context"
	"errors"
	"testing"
	"time"

	infraegress "github.com/chenyme/grok2api/backend/internal/infra/egress"
)

// TestStatsigMissingMetaBacksOffAcrossPaths reproduces the quota-refresh storm
// when both current Grok pages no longer contain the verification meta.
func TestStatsigMissingMetaBacksOffAcrossPaths(t *testing.T) {
	signer := newStatsigSigner()
	now := time.Now()
	signer.now = func() time.Time { return now }
	calls := 0
	signer.fetchMeta = func(context.Context, string, string, *infraegress.Lease) (string, error) {
		calls++
		return "", ErrStatsigMetaMissing
	}
	for _, path := range []string{"/chat", "/quota", "/media", "/chat"} {
		_, _, err := signer.Sign(context.Background(), "https://grok.example", "https://sign.example", "", nil, "POST", path)
		if !errors.Is(err, ErrStatsigMetaMissing) {
			t.Fatalf("error = %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("metadata fetches = %d, want 1 during shared backoff", calls)
	}
	now = now.Add(5*time.Minute + time.Second)
	_, _, _ = signer.Sign(context.Background(), "https://grok.example", "https://sign.example", "", nil, "POST", "/chat")
	if calls != 2 {
		t.Fatalf("metadata fetches after expiry = %d", calls)
	}
	signer.Clear()
	_, _, _ = signer.Sign(context.Background(), "https://grok.example", "https://sign.example", "", nil, "POST", "/chat")
	if calls != 3 {
		t.Fatalf("metadata fetches after config reset = %d", calls)
	}
}

// TestStatsigDoesNotCacheProxyFailure preserves recovery via another account/egress.
func TestStatsigDoesNotCacheProxyFailure(t *testing.T) {
	signer := newStatsigSigner()
	calls := 0
	signer.fetchMeta = func(context.Context, string, string, *infraegress.Lease) (string, error) {
		calls++
		return "", errors.New("Grok index 返回 403")
	}
	for range 2 {
		_, _, _ = signer.Sign(context.Background(), "https://grok.example", "https://sign.example", "", nil, "POST", "/chat")
	}
	if calls != 2 {
		t.Fatalf("metadata fetches = %d, proxy failure must not poison shared state", calls)
	}
}
