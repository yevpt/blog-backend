package bridge

import (
	"strings"

	domain "github.com/vpt/blog-backend/internal/oauth"
)

const githubExchangePath = "/internal/oauth/github/exchange"

// ExchangeRequest 是发往海外 Bridge 的换码请求体。
type ExchangeRequest struct {
	Code         string `json:"code"`
	RedirectURI  string `json:"redirectUri"`
	CodeVerifier string `json:"codeVerifier"`
}

// ProfileResponse 是 Bridge 返回的标准化第三方用户资料。
type ProfileResponse struct {
	Provider        string `json:"provider"`
	ProviderUserID  string `json:"providerUserId"`
	Login           string `json:"login"`
	Name            string `json:"name"`
	AvatarURL       string `json:"avatarUrl"`
	HTMLURL         string `json:"htmlUrl"`
	BlogURL         string `json:"blogUrl"`
	Email           string `json:"email"`
	EmailVerified   bool   `json:"emailVerified"`
}

// ToProfile 映射为本站 OAuth 统一 Profile。
func (r ProfileResponse) ToProfile() *domain.Profile {
	source := strings.TrimSpace(r.Provider)
	if source == "" {
		source = "github"
	}

	nickname := strPtr(r.Name)
	if nickname == nil {
		nickname = strPtr(r.Login)
	}

	var email *string
	if trimmed := strings.TrimSpace(r.Email); trimmed != "" && r.EmailVerified {
		email = &trimmed
	}

	return &domain.Profile{
		Source:    source,
		UUID:      strings.TrimSpace(r.ProviderUserID),
		Email:     email,
		Nickname:  nickname,
		AvatarURL: strPtr(r.AvatarURL),
		BlogURL:   strPtr(r.BlogURL),
	}
}

func strPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
