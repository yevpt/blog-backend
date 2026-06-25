package bridge_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/oauth/bridge"
)

func TestClient_ExchangeGitHub_Success(t *testing.T) {
	secret := "bridge-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/internal/oauth/github/exchange", r.URL.Path)

		timestamp := r.Header.Get("X-Bridge-Timestamp")
		require.NotEmpty(t, timestamp)

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, signTest(secret, timestamp, body), r.Header.Get("X-Bridge-Signature"))
		assert.JSONEq(t, `{"code":"auth-code","redirectUri":"https://api.example.com/oauth/github/callback","codeVerifier":"pkce-verifier"}`, string(body))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"provider":"github",
			"providerUserId":"42",
			"login":"octocat",
			"name":"The Octocat",
			"avatarUrl":"https://avatars.githubusercontent.com/u/42",
			"blogUrl":"https://octocat.blog",
			"email":"octocat@example.com",
			"emailVerified":true
		}`))
	}))
	t.Cleanup(server.Close)

	client := bridge.NewClient(server.URL, secret)
	profile, err := client.ExchangeGitHub(context.Background(), bridge.ExchangeRequest{
		Code:         "auth-code",
		RedirectURI:  "https://api.example.com/oauth/github/callback",
		CodeVerifier: "pkce-verifier",
	})
	require.NoError(t, err)
	assert.Equal(t, "github", profile.Source)
	assert.Equal(t, "42", profile.UUID)
	require.NotNil(t, profile.Nickname)
	assert.Equal(t, "The Octocat", *profile.Nickname)
	require.NotNil(t, profile.Email)
	assert.Equal(t, "octocat@example.com", *profile.Email)
}

func TestClient_ExchangeGitHub_RejectsBadSignature(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	client := bridge.NewClient(server.URL, "bridge-secret")
	_, err := client.ExchangeGitHub(context.Background(), bridge.ExchangeRequest{Code: "auth-code"})
	assert.ErrorIs(t, err, bridge.ErrUnauthorized)
}

func TestClient_ExchangeGitHub_RejectsInvalidCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)

	client := bridge.NewClient(server.URL, "bridge-secret")
	_, err := client.ExchangeGitHub(context.Background(), bridge.ExchangeRequest{Code: "bad-code"})
	assert.ErrorIs(t, err, bridge.ErrExchangeFailed)
}

func TestNewClient_ReturnsNilWhenBridgeNotConfigured(t *testing.T) {
	assert.Nil(t, bridge.NewClient("", "secret"))
	assert.Nil(t, bridge.NewClient("https://bridge.example.com", ""))
}

func TestProfileResponse_ToProfile_UsesLoginWhenNameMissing(t *testing.T) {
	profile := bridge.ProfileResponse{
		Provider:       "github",
		ProviderUserID: "7",
		Login:          "ghost",
	}.ToProfile()
	require.NotNil(t, profile.Nickname)
	assert.Equal(t, "ghost", *profile.Nickname)
}

func TestProfileResponse_ToProfile_IgnoresUnverifiedEmail(t *testing.T) {
	profile := bridge.ProfileResponse{
		Provider:       "github",
		ProviderUserID: "7",
		Email:          "hidden@example.com",
		EmailVerified:  false,
	}.ToProfile()
	assert.Nil(t, profile.Email)
}

func signTest(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestClient_ExchangeGitHub_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client := bridge.NewClient(server.URL, "bridge-secret")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	t.Cleanup(cancel)

	_, err := client.ExchangeGitHub(ctx, bridge.ExchangeRequest{Code: "auth-code"})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "超时") || strings.Contains(err.Error(), "不可用"))
}
