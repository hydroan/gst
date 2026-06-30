package servicemfa

import (
	"context"
	"testing"
	"time"

	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

type totpBindChallengeTestCache struct {
	items map[string]totpBindChallenge
}

func newTOTPBindChallengeTestCache() *totpBindChallengeTestCache {
	return &totpBindChallengeTestCache{items: make(map[string]totpBindChallenge)}
}

func (c *totpBindChallengeTestCache) Get(key string) (totpBindChallenge, error) {
	value, ok := c.items[key]
	if !ok {
		return totpBindChallenge{}, types.ErrEntryNotFound
	}
	return value, nil
}

func (c *totpBindChallengeTestCache) Peek(key string) (totpBindChallenge, error) {
	return c.Get(key)
}

func (c *totpBindChallengeTestCache) Set(key string, value totpBindChallenge, _ time.Duration) error {
	c.items[key] = value
	return nil
}

func (c *totpBindChallengeTestCache) Delete(key string) error {
	delete(c.items, key)
	return nil
}

func (c *totpBindChallengeTestCache) Exists(key string) bool {
	_, ok := c.items[key]
	return ok
}

func (c *totpBindChallengeTestCache) Len() int {
	return len(c.items)
}

func (c *totpBindChallengeTestCache) Clear() {
	clear(c.items)
}

func (c *totpBindChallengeTestCache) WithContext(context.Context) types.Cache[totpBindChallenge] {
	return c
}

func TestIssueTOTPBindChallengeUsesOpaqueRandomToken(t *testing.T) {
	cache := newTOTPBindChallengeTestCache()
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	restore := stubTOTPBindChallengeGlobals(cache, now)
	t.Cleanup(restore)

	firstID, firstChallenge, err := issueTOTPBindChallenge(context.Background(), totpBindChallenge{
		UserID:    " user-1 ",
		SessionID: " session-1 ",
		Username:  " user@example.com ",
		Secret:    " secret ",
	})
	require.NoError(t, err)
	secondID, _, err := issueTOTPBindChallenge(context.Background(), totpBindChallenge{
		UserID:    "user-1",
		SessionID: "session-1",
		Username:  "user@example.com",
		Secret:    "secret",
	})
	require.NoError(t, err)

	require.Len(t, firstID, 43)
	require.Regexp(t, `^[A-Za-z0-9_-]+$`, firstID)
	require.NotRegexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, firstID)
	require.NotEqual(t, firstID, secondID)

	loaded, err := loadTOTPBindChallenge(context.Background(), firstID)
	require.NoError(t, err)
	require.Equal(t, firstChallenge, loaded)
	require.Equal(t, "user-1", loaded.UserID)
	require.Equal(t, "session-1", loaded.SessionID)
	require.Equal(t, "user@example.com", loaded.Username)
	require.Equal(t, "secret", loaded.Secret)
	require.Equal(t, now, loaded.IssuedAt)
	require.Equal(t, now.Add(totpBindChallengeTTL), loaded.ExpiresAt)
}

func stubTOTPBindChallengeGlobals(cache types.Cache[totpBindChallenge], now time.Time) func() {
	originalCache := totpBindChallengeCache
	originalNow := totpBindChallengeNow
	totpBindChallengeCache = func() types.Cache[totpBindChallenge] { return cache }
	totpBindChallengeNow = func() time.Time { return now }
	return func() {
		totpBindChallengeCache = originalCache
		totpBindChallengeNow = originalNow
	}
}
