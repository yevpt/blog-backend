package analytics_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

func TestCollectTokenVerifier_EmptySecretAllows(t *testing.T) {
	v := svc.NewCollectTokenVerifier("", time.Minute, func() time.Time { return time.Unix(1000, 0) })

	ok, reason := v.Verify("")

	assert.True(t, ok)
	assert.Empty(t, reason)
}

func TestCollectTokenVerifier_VerifiesSignedToken(t *testing.T) {
	now := time.Unix(1000, 0)
	v := svc.NewCollectTokenVerifier("secret", time.Minute, func() time.Time { return now })
	token, err := svc.SignCollectTokenForTest("secret", now.Add(time.Minute))
	require.NoError(t, err)

	ok, reason := v.Verify(token)

	assert.True(t, ok)
	assert.Empty(t, reason)
}

func TestCollectTokenVerifier_RejectsMissingToken(t *testing.T) {
	v := svc.NewCollectTokenVerifier("secret", time.Minute, func() time.Time { return time.Unix(1000, 0) })

	ok, reason := v.Verify("")

	assert.False(t, ok)
	assert.Equal(t, "collect_token_missing", reason)
}

func TestCollectTokenVerifier_RejectsExpiredAndTampered(t *testing.T) {
	now := time.Unix(1000, 0)
	v := svc.NewCollectTokenVerifier("secret", time.Minute, func() time.Time { return now })
	expired, err := svc.SignCollectTokenForTest("secret", now.Add(-time.Second))
	require.NoError(t, err)

	ok, reason := v.Verify(expired)
	assert.False(t, ok)
	assert.Equal(t, "collect_token_expired", reason)

	ok, reason = v.Verify(expired + "x")
	assert.False(t, ok)
	assert.Equal(t, "collect_token_invalid", reason)
}

func TestCollectTokenVerifier_RejectsInvalidAudience(t *testing.T) {
	now := time.Unix(1000, 0)
	v := svc.NewCollectTokenVerifier("secret", time.Minute, func() time.Time { return now })
	token := signCollectToken(t, "secret", map[string]any{
		"aud": "other",
		"exp": now.Add(time.Minute).Unix(),
	})

	ok, reason := v.Verify(token)

	assert.False(t, ok)
	assert.Equal(t, "collect_token_invalid", reason)
}

func TestCollectTokenVerifier_RejectsTokenBeyondTTL(t *testing.T) {
	now := time.Unix(1000, 0)
	v := svc.NewCollectTokenVerifier("secret", time.Minute, func() time.Time { return now })
	token, err := svc.SignCollectTokenForTest("secret", now.Add(2*time.Minute))
	require.NoError(t, err)

	ok, reason := v.Verify(token)

	assert.False(t, ok)
	assert.Equal(t, "collect_token_invalid", reason)
}

func signCollectToken(t *testing.T, secret string, payload map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	encodedPayload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(secret))
	_, err = mac.Write([]byte(encodedPayload))
	require.NoError(t, err)

	return encodedPayload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
