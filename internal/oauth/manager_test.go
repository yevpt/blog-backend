package oauth_test

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/oauth"
)

type fakeProvider struct {
	source          string
	authCodeURL     string
	authErr         error
	exchangeErr     error
	fetchProfileErr error
	gotAuthOpts     oauth.AuthCodeOptions
	gotCode         string
	gotVerifier     string
}

func (p *fakeProvider) Source() string { return p.source }

func (p *fakeProvider) AuthCodeURL(opts oauth.AuthCodeOptions) (string, error) {
	p.gotAuthOpts = opts
	if p.authErr != nil {
		return "", p.authErr
	}
	u, err := url.Parse(p.authCodeURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("state", opts.State)
	q.Set("code_challenge_source", opts.Verifier)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (p *fakeProvider) Exchange(_ context.Context, code string, verifier string) (*oauth.TokenSet, error) {
	p.gotCode = code
	p.gotVerifier = verifier
	if p.exchangeErr != nil {
		return nil, p.exchangeErr
	}
	return &oauth.TokenSet{AccessToken: "access-token"}, nil
}

func (p *fakeProvider) FetchProfile(_ context.Context, _ *oauth.TokenSet) (*oauth.Profile, error) {
	if p.fetchProfileErr != nil {
		return nil, p.fetchProfileErr
	}
	return &oauth.Profile{Source: p.source, UUID: "remote-user-id"}, nil
}

func TestManager_AuthorizeAndCallback(t *testing.T) {
	store, _ := newStateStore(t, 10*time.Minute)
	provider := &fakeProvider{source: "github", authCodeURL: "https://github.com/login/oauth/authorize"}
	manager := oauth.NewManager(store, []oauth.Provider{provider})
	ctx := context.Background()

	authURL, err := manager.Authorize(ctx, "github", oauth.FlowContext{
		Action: oauth.ActionLogin,
	})
	require.NoError(t, err)
	require.NotEmpty(t, provider.gotAuthOpts.State)
	require.NotEmpty(t, provider.gotAuthOpts.Verifier)
	assert.Contains(t, authURL, "state=")

	result, err := manager.Callback(ctx, "github", "callback-code", provider.gotAuthOpts.State)
	require.NoError(t, err)

	assert.Equal(t, "callback-code", provider.gotCode)
	assert.Equal(t, provider.gotAuthOpts.Verifier, provider.gotVerifier)
	assert.Equal(t, oauth.ActionLogin, result.Flow.Action)
	assert.Equal(t, "github", result.Profile.Source)
	assert.Equal(t, "remote-user-id", result.Profile.UUID)
	assert.Equal(t, "access-token", result.Token.AccessToken)
}

func TestManager_AuthorizeRejectsUnknownProvider(t *testing.T) {
	store, _ := newStateStore(t, 10*time.Minute)
	manager := oauth.NewManager(store, nil)

	_, err := manager.Authorize(context.Background(), "github", oauth.FlowContext{Action: oauth.ActionLogin})

	assert.ErrorIs(t, err, oauth.ErrProviderNotEnabled)
}

func TestManager_SourcesReturnsSortedProviderNames(t *testing.T) {
	manager := oauth.NewManager(nil, []oauth.Provider{
		&fakeProvider{source: "gitee"},
		&fakeProvider{source: "github"},
	})

	assert.Equal(t, []string{"gitee", "github"}, manager.Sources())
}

func TestManager_CallbackRejectsSourceMismatch(t *testing.T) {
	store, _ := newStateStore(t, 10*time.Minute)
	manager := oauth.NewManager(store, []oauth.Provider{
		&fakeProvider{source: "github", authCodeURL: "https://github.example.com/auth"},
		&fakeProvider{source: "gitee", authCodeURL: "https://gitee.example.com/auth"},
	})
	ctx := context.Background()

	state, err := store.Create(ctx, oauth.FlowContext{
		Source:   "github",
		Action:   oauth.ActionLogin,
		Verifier: "pkce-verifier",
	})
	require.NoError(t, err)

	_, err = manager.Callback(ctx, "gitee", "callback-code", state)

	assert.ErrorIs(t, err, oauth.ErrStateSourceMismatch)
}

func TestManager_CallbackConsumesStateOnlyOnce(t *testing.T) {
	store, _ := newStateStore(t, 10*time.Minute)
	provider := &fakeProvider{source: "github", authCodeURL: "https://github.example.com/auth"}
	manager := oauth.NewManager(store, []oauth.Provider{provider})
	ctx := context.Background()

	_, err := manager.Authorize(ctx, "github", oauth.FlowContext{Action: oauth.ActionLogin})
	require.NoError(t, err)
	state := provider.gotAuthOpts.State

	_, err = manager.Callback(ctx, "github", "callback-code", state)
	require.NoError(t, err)

	_, err = manager.Callback(ctx, "github", "callback-code", state)
	assert.ErrorIs(t, err, oauth.ErrInvalidState)
}

func TestManager_CallbackReturnsProviderError(t *testing.T) {
	store, _ := newStateStore(t, 10*time.Minute)
	wantErr := errors.New("exchange failed")
	provider := &fakeProvider{
		source:      "github",
		authCodeURL: "https://github.example.com/auth",
		exchangeErr: wantErr,
	}
	manager := oauth.NewManager(store, []oauth.Provider{provider})
	ctx := context.Background()

	_, err := manager.Authorize(ctx, "github", oauth.FlowContext{Action: oauth.ActionLogin})
	require.NoError(t, err)

	_, err = manager.Callback(ctx, "github", "callback-code", provider.gotAuthOpts.State)
	assert.ErrorIs(t, err, wantErr)
}
