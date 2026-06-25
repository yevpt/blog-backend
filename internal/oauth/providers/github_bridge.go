package providers

import (
	"context"
	"errors"
	"net"
	"net/url"
	"syscall"

	domain "github.com/vpt/blog-backend/internal/oauth"
	"github.com/vpt/blog-backend/internal/oauth/bridge"
	gooauth2 "golang.org/x/oauth2"
)

func (p *GitHubProvider) exchangeViaBridge(ctx context.Context, code, verifier string) (*domain.TokenSet, *domain.Profile, error) {
	profile, err := p.bridge.ExchangeGitHub(ctx, bridge.ExchangeRequest{
		Code:         code,
		RedirectURI:  p.cfg.RedirectURI,
		CodeVerifier: verifier,
	})
	if err != nil {
		return nil, nil, err
	}
	return &domain.TokenSet{}, profile, nil
}

func (p *GitHubProvider) exchangeDirect(ctx context.Context, code, verifier string) (*domain.TokenSet, *domain.Profile, error) {
	token, err := p.Exchange(ctx, code, verifier)
	if err != nil {
		return nil, nil, err
	}
	profile, err := p.FetchProfile(ctx, token)
	if err != nil {
		return nil, nil, err
	}
	return token, profile, nil
}

func IsDirectExchangeRetryable(err error) bool {
	if err == nil {
		return false
	}
	var retrieveErr *gooauth2.RetrieveError
	if errors.As(err, &retrieveErr) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if IsDirectExchangeRetryable(urlErr.Err) {
			return true
		}
	}
	return errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ENETUNREACH) ||
		errors.Is(err, syscall.EHOSTUNREACH)
}
