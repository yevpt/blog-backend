package providers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/vpt/blog-backend/internal/oauth/bridge"
	domain "github.com/vpt/blog-backend/internal/oauth"
	"github.com/vpt/blog-backend/pkg/config"
	gooauth2 "golang.org/x/oauth2"
)

const (
	githubSource   = "github"
	githubAuthURL  = "https://github.com/login/oauth/authorize"
	githubTokenURL = "https://github.com/login/oauth/access_token"
	githubUserURL  = "https://api.github.com/user"
)

// GitHubProvider 封装 GitHub OAuth2 登录差异。
type GitHubProvider struct {
	cfg    config.OAuthProviderConfig
	oauth  *gooauth2.Config
	client httpClient
	bridge bridge.Client
}

// NewGitHubProvider 创建 GitHub OAuth provider。
func NewGitHubProvider(cfg config.OAuthProviderConfig) *GitHubProvider {
	applyGitHubDefaults(&cfg)
	return &GitHubProvider{
		cfg: cfg,
		oauth: &gooauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURI,
			Scopes:       cfg.Scopes,
			Endpoint: gooauth2.Endpoint{
				AuthURL:  cfg.AuthURL,
				TokenURL: cfg.TokenURL,
			},
		},
		client: newProviderHTTPClient(),
		bridge: bridge.NewClient(cfg.BridgeURL, cfg.BridgeSecret),
	}
}

// Source 返回本站内部统一使用的平台标识。
func (p *GitHubProvider) Source() string {
	return githubSource
}

// AuthCodeURL 生成 GitHub 授权地址，并携带 state 与 PKCE challenge。
func (p *GitHubProvider) AuthCodeURL(opts domain.AuthCodeOptions) (string, error) {
	authOpts := []gooauth2.AuthCodeOption{}
	if opts.Verifier != "" {
		authOpts = append(authOpts, gooauth2.S256ChallengeOption(opts.Verifier))
	}
	return p.oauth.AuthCodeURL(opts.State, authOpts...), nil
}

// ExchangeProfile 换码并拉取用户资料；策略由 bridge_mode 与 Bridge 是否配置共同决定。
func (p *GitHubProvider) ExchangeProfile(ctx context.Context, code string, verifier string) (*domain.TokenSet, *domain.Profile, error) {
	switch p.cfg.EffectiveGitHubBridgeMode() {
	case config.GitHubBridgeModeBridgeOnly:
		return p.exchangeViaBridge(ctx, code, verifier)
	case config.GitHubBridgeModeFallback:
		token, err := p.Exchange(ctx, code, verifier)
		if err == nil {
			profile, fetchErr := p.FetchProfile(ctx, token)
			if fetchErr != nil {
				return nil, nil, fetchErr
			}
			return token, profile, nil
		}
		if IsDirectExchangeRetryable(err) {
			return p.exchangeViaBridge(ctx, code, verifier)
		}
		return nil, nil, err
	default:
		return p.exchangeDirect(ctx, code, verifier)
	}
}

// Exchange 使用 callback code 换取 GitHub access token。
func (p *GitHubProvider) Exchange(ctx context.Context, code string, verifier string) (*domain.TokenSet, error) {
	opts := []gooauth2.AuthCodeOption{}
	if verifier != "" {
		opts = append(opts, gooauth2.VerifierOption(verifier))
	}
	token, err := p.oauth.Exchange(ctx, code, opts...)
	if err != nil {
		return nil, err
	}
	return tokenSetFromOAuth2(token), nil
}

// FetchProfile 拉取 GitHub 用户资料，并补齐主邮箱。
func (p *GitHubProvider) FetchProfile(ctx context.Context, token *domain.TokenSet) (*domain.Profile, error) {
	var user githubUser
	if err := getJSONWithBearer(ctx, p.client, "GitHub", p.cfg.UserURL, token.AccessToken, &user); err != nil {
		return nil, err
	}

	email := strPtr(user.Email)
	if email == nil {
		if loadedEmail, err := p.fetchPrimaryEmail(ctx, token.AccessToken); err == nil {
			email = loadedEmail
		}
	}

	nickname := strPtr(user.Name)
	if nickname == nil {
		nickname = strPtr(user.Login)
	}

	return &domain.Profile{
		Source:    githubSource,
		UUID:      user.ID.String(),
		Email:     email,
		Nickname:  nickname,
		AvatarURL: strPtr(user.AvatarURL),
		BlogURL:   strPtr(user.Blog),
	}, nil
}

func (p *GitHubProvider) fetchPrimaryEmail(ctx context.Context, accessToken string) (*string, error) {
	var emails []githubEmail
	if err := getJSONWithBearer(ctx, p.client, "GitHub", p.cfg.UserURL+"/emails", accessToken, &emails); err != nil {
		return nil, err
	}
	for _, email := range emails {
		if email.Primary && email.Verified && strings.TrimSpace(email.Email) != "" {
			return &email.Email, nil
		}
	}
	return nil, nil
}

func applyGitHubDefaults(cfg *config.OAuthProviderConfig) {
	if cfg.AuthURL == "" {
		cfg.AuthURL = githubAuthURL
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = githubTokenURL
	}
	if cfg.UserURL == "" {
		cfg.UserURL = githubUserURL
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"read:user", "user:email"}
	}
}

type githubUser struct {
	ID        json.Number `json:"id"`
	Login     string      `json:"login"`
	Name      string      `json:"name"`
	Email     string      `json:"email"`
	AvatarURL string      `json:"avatar_url"`
	Blog      string      `json:"blog"`
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}
