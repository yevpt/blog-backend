package auth_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/vpt/blog-backend/internal/dto"
	authhandler "github.com/vpt/blog-backend/internal/handler/auth"
	authservice "github.com/vpt/blog-backend/internal/service/auth"
	"github.com/vpt/blog-backend/pkg/response"
)

// stubAuthService 测试用 stub
type stubAuthService struct {
	sendCodeErr  error
	registerResp *dto.UserResp
	registerErr  error
	loginResp    *dto.LoginResp
	loginErr     error
	refreshResp  *dto.TokenResp
	refreshErr   error
}

func (s *stubAuthService) SendCode(email, ip string, captchaToken string) error {
	return s.sendCodeErr
}
func (s *stubAuthService) Register(req *dto.RegisterReq) (*dto.UserResp, error) {
	return s.registerResp, s.registerErr
}
func (s *stubAuthService) Login(req *dto.LoginReq, ip string) (*dto.LoginResp, error) {
	return s.loginResp, s.loginErr
}
func (s *stubAuthService) Refresh(rt string) (*dto.TokenResp, error) {
	return s.refreshResp, s.refreshErr
}

func newTestRouter(svc authservice.AuthService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := authhandler.NewAuthHandler(svc)
	r.POST("/auth/send-code", h.SendCode)
	r.POST("/auth/register", h.Register)
	r.POST("/auth/login", h.Login)
	r.POST("/auth/refresh", h.Refresh)
	return r
}

func TestAuthHandler_SendCode_Success(t *testing.T) {
	r := newTestRouter(&stubAuthService{})
	body, _ := json.Marshal(map[string]string{
		"email":         "user@example.com",
		"captcha_token": "captcha-token",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/send-code", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestAuthHandler_SendCode_InvalidEmail(t *testing.T) {
	r := newTestRouter(&stubAuthService{})
	body, _ := json.Marshal(map[string]string{
		"email":         "notanemail",
		"captcha_token": "captcha-token",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/send-code", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, response.CodeBadRequest, resp.Code)
}

func TestAuthHandler_SendCode_MissingCaptchaToken(t *testing.T) {
	r := newTestRouter(&stubAuthService{})
	body, _ := json.Marshal(map[string]string{"email": "user@example.com"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/send-code", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, response.CodeBadRequest, resp.Code)
}

func TestAuthHandler_SendCode_TooManyRequests(t *testing.T) {
	r := newTestRouter(&stubAuthService{sendCodeErr: authservice.ErrTooManyRequests})
	body, _ := json.Marshal(map[string]string{
		"email":         "user@example.com",
		"captcha_token": "captcha-token",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/send-code", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestAuthHandler_Register_Success(t *testing.T) {
	nick := "alice"
	email := "alice@example.com"
	stub := &stubAuthService{
		registerResp: &dto.UserResp{ID: 1, Username: email, Email: &email, Nickname: &nick},
	}
	r := newTestRouter(stub)
	body, _ := json.Marshal(map[string]string{
		"email": email, "password": "password123", "code": "123456",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestAuthHandler_Register_ShortPasswordReturnsReadableMessage(t *testing.T) {
	r := newTestRouter(&stubAuthService{})
	body, _ := json.Marshal(map[string]string{
		"email": "alice@example.com", "password": "123456", "code": "123456",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
	assert.Equal(t, "密码长度不能短于 8 个字符", resp.Message)
}

func TestAuthHandler_SendCode_InvalidJSONReturnsReadableMessage(t *testing.T) {
	r := newTestRouter(&stubAuthService{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/send-code", bytes.NewReader([]byte(`{"email":}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
	assert.Equal(t, "请求体必须是合法的 JSON", resp.Message)
}

func TestAuthHandler_Login_Success(t *testing.T) {
	stub := &stubAuthService{
		loginResp: &dto.LoginResp{
			AccessToken:  "access.token.here",
			RefreshToken: "refresh.token.here",
			ExpiresIn:    7200,
		},
	}
	r := newTestRouter(stub)
	body, _ := json.Marshal(map[string]string{
		"identifier": "user@example.com", "password": "password123",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthHandler_Login_Disabled(t *testing.T) {
	stub := &stubAuthService{loginErr: authservice.ErrUserDisabled}
	r := newTestRouter(stub)
	body, _ := json.Marshal(map[string]string{
		"identifier": "user@example.com", "password": "password123",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, response.CodeForbidden, resp.Code)
	assert.Equal(t, "账号已被禁用", resp.Message)
}

func TestAuthHandler_Login_UserNotFound(t *testing.T) {
	stub := &stubAuthService{loginErr: authservice.ErrUserNotFound}
	r := newTestRouter(stub)
	body, _ := json.Marshal(map[string]string{
		"identifier": "nobody", "password": "password123",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, response.CodeUnauth, resp.Code)
	assert.Equal(t, "账号不存在", resp.Message)
}

func TestAuthHandler_Login_WrongPassword(t *testing.T) {
	stub := &stubAuthService{loginErr: authservice.ErrWrongPassword}
	r := newTestRouter(stub)
	body, _ := json.Marshal(map[string]string{
		"identifier": "user@example.com", "password": "wrongpassword",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, response.CodeUnauth, resp.Code)
	assert.Equal(t, "密码错误", resp.Message)
}

func TestAuthHandler_Login_InternalError(t *testing.T) {
	stub := &stubAuthService{loginErr: errors.New("load roles failed")}
	r := newTestRouter(stub)
	body, _ := json.Marshal(map[string]string{
		"identifier": "user@example.com", "password": "password123",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, response.CodeServerError, resp.Code)
	assert.Equal(t, "服务器内部错误", resp.Message)
}

func TestAuthHandler_Refresh_InvalidToken(t *testing.T) {
	stub := &stubAuthService{refreshErr: errors.New("token 无效或已过期")}
	r := newTestRouter(stub)
	body, _ := json.Marshal(map[string]string{"refresh_token": "bad.token"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
