package oauth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/dto"
	handleroauth "github.com/vpt/blog-backend/internal/handler/oauth"
	domain "github.com/vpt/blog-backend/internal/oauth"
	serviceoauth "github.com/vpt/blog-backend/internal/service/oauth"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

type stubOAuthService struct {
	authorizeResp *dto.OAuthAuthorizeResp
	callbackResp  *dto.OAuthCallbackResp
	err           error
	gotUserID     uint
	gotAction     domain.Action
}

func (s *stubOAuthService) Authorize(ctx context.Context, source string, action domain.Action, userID uint, redirectURI string) (*dto.OAuthAuthorizeResp, error) {
	s.gotUserID = userID
	s.gotAction = action
	return s.authorizeResp, s.err
}

func (s *stubOAuthService) Providers(ctx context.Context) []string {
	return []string{"github"}
}

func (s *stubOAuthService) Callback(ctx context.Context, source string, code string, state string) (*dto.OAuthCallbackResp, error) {
	return s.callbackResp, s.err
}

func (s *stubOAuthService) ListBindings(ctx context.Context, userID uint) ([]dto.OAuthBindingResp, error) {
	return nil, s.err
}

func (s *stubOAuthService) Unbind(ctx context.Context, userID uint, source string) error {
	return s.err
}

func newOAuthRouter(svc serviceoauth.OAuthService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handleroauth.NewOAuthHandler(svc)
	r.GET("/oauth/:source/authorize", h.Authorize)
	r.GET("/oauth/:source/callback", h.Callback)
	return r
}

func TestOAuthHandler_AuthorizeSuccess(t *testing.T) {
	svc := &stubOAuthService{
		authorizeResp: &dto.OAuthAuthorizeResp{AuthorizeURL: "https://github.example.com/auth"},
	}
	r := newOAuthRouter(svc)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/oauth/github/authorize?action=login", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, domain.ActionLogin, svc.gotAction)

	var resp struct {
		Code int                    `json:"code"`
		Data dto.OAuthAuthorizeResp `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeOK, resp.Code)
	assert.Equal(t, "https://github.example.com/auth", resp.Data.AuthorizeURL)
}

func TestOAuthHandler_AuthorizeBindRequiresLogin(t *testing.T) {
	svc := &stubOAuthService{err: serviceoauth.ErrLoginRequired}
	r := newOAuthRouter(svc)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/oauth/github/authorize?action=bind", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestOAuthHandler_AuthorizePassesClaimsUserID(t *testing.T) {
	svc := &stubOAuthService{
		authorizeResp: &dto.OAuthAuthorizeResp{AuthorizeURL: "https://github.example.com/auth"},
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handleroauth.NewOAuthHandler(svc)
	r.Use(func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 7})
		c.Next()
	})
	r.GET("/oauth/:source/authorize", h.Authorize)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/oauth/github/authorize?action=bind", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(7), svc.gotUserID)
}

func TestOAuthHandler_CallbackSuccess(t *testing.T) {
	svc := &stubOAuthService{
		callbackResp: &dto.OAuthCallbackResp{
			Action: string(domain.ActionLogin),
			Login:  &dto.LoginResp{AccessToken: "access-token"},
		},
	}
	r := newOAuthRouter(svc)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/oauth/github/callback?code=code&state=state", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int                   `json:"code"`
		Data dto.OAuthCallbackResp `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "access-token", resp.Data.Login.AccessToken)
}

func TestOAuthHandler_CallbackBusinessError(t *testing.T) {
	svc := &stubOAuthService{err: serviceoauth.ErrSocialIdentityBound}
	r := newOAuthRouter(svc)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/oauth/github/callback?code=code&state=state", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
}
