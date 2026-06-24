package auth_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
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
	sendCodeErr    error
	resetCodeEmail string
	resetCodeToken string
	resetCodeErr   error
	resetReq       *dto.PasswordResetReq
	resetErr       error
	registerResp   *dto.LoginResp
	registerErr    error
	loginResp      *dto.LoginResp
	loginErr       error
	adminLoginReq  *dto.AdminLoginReq
	adminLoginResp *dto.LoginResp
	adminLoginErr  error
	refreshResp    *dto.TokenResp
	refreshErr     error
}

func (s *stubAuthService) SendCode(email, ip string, captchaToken string) error {
	return s.sendCodeErr
}
func (s *stubAuthService) SendPasswordResetCode(email, ip string, captchaToken string) error {
	s.resetCodeEmail = email
	s.resetCodeToken = captchaToken
	return s.resetCodeErr
}
func (s *stubAuthService) ResetPassword(req *dto.PasswordResetReq) error {
	s.resetReq = req
	return s.resetErr
}
func (s *stubAuthService) Register(req *dto.RegisterReq, avatar *dto.UploadedImageFile) (*dto.LoginResp, error) {
	return s.registerResp, s.registerErr
}
func (s *stubAuthService) Login(req *dto.LoginReq, ip string) (*dto.LoginResp, error) {
	return s.loginResp, s.loginErr
}
func (s *stubAuthService) AdminLogin(req *dto.AdminLoginReq, ip string) (*dto.LoginResp, error) {
	s.adminLoginReq = req
	return s.adminLoginResp, s.adminLoginErr
}
func (s *stubAuthService) Refresh(rt string) (*dto.TokenResp, error) {
	return s.refreshResp, s.refreshErr
}

func newTestRouter(svc authservice.AuthService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := authhandler.NewAuthHandler(svc)
	r.POST("/auth/send-code", h.SendCode)
	r.POST("/auth/password-reset/code", h.SendPasswordResetCode)
	r.POST("/auth/password-reset", h.ResetPassword)
	r.POST("/auth/register", h.Register)
	r.POST("/auth/login", h.Login)
	r.POST("/admin/auth/login", h.AdminLogin)
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

func TestAuthHandler_SendPasswordResetCode_Success(t *testing.T) {
	stub := &stubAuthService{}
	r := newTestRouter(stub)
	body, _ := json.Marshal(map[string]string{
		"email":         "user@example.com",
		"captcha_token": "captcha-token",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/password-reset/code", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, response.CodeOK, resp.Code)
	assert.Equal(t, "user@example.com", stub.resetCodeEmail)
	assert.Equal(t, "captcha-token", stub.resetCodeToken)
}

func TestAuthHandler_ResetPassword_Success(t *testing.T) {
	stub := &stubAuthService{}
	r := newTestRouter(stub)
	body, _ := json.Marshal(map[string]string{
		"email":        "user@example.com",
		"code":         "123456",
		"new_password": "new-password",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/password-reset", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, response.CodeOK, resp.Code)
	assert.Equal(t, "user@example.com", stub.resetReq.Email)
	assert.Equal(t, "123456", stub.resetReq.Code)
}

func TestAuthHandler_Register_Success(t *testing.T) {
	nick := "alice"
	email := "alice@example.com"
	stub := &stubAuthService{
		registerResp: &dto.LoginResp{
			AccessToken:  "access",
			RefreshToken: "refresh",
			ExpiresIn:    7200,
			User:         dto.UserResp{ID: 1, Username: email, Email: &email, Nickname: &nick},
		},
	}
	r := newTestRouter(stub)

	body, contentType := newRegisterMultipartBody(email, "password123", "123456", &nick, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/register", body)
	req.Header.Set("Content-Type", contentType)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestAuthHandler_Register_ShortPasswordReturnsReadableMessage(t *testing.T) {
	r := newTestRouter(&stubAuthService{})
	body, contentType := newRegisterMultipartBody("alice@example.com", "123456", "123456", nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/register", body)
	req.Header.Set("Content-Type", contentType)
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

func TestAuthHandler_AdminLogin_SuccessUsesUsernamePassword(t *testing.T) {
	stub := &stubAuthService{
		adminLoginResp: &dto.LoginResp{
			AccessToken:  "access.token.here",
			RefreshToken: "refresh.token.here",
			ExpiresIn:    7200,
		},
	}
	r := newTestRouter(stub)
	body, _ := json.Marshal(map[string]string{
		"username": "root", "password": "password123",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "root", stub.adminLoginReq.Username)
}

func TestAuthHandler_AdminLogin_RejectsNonAdmin(t *testing.T) {
	stub := &stubAuthService{adminLoginErr: authservice.ErrAdminRequired}
	r := newTestRouter(stub)
	body, _ := json.Marshal(map[string]string{
		"username": "alice", "password": "password123",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, response.CodeForbidden, resp.Code)
	assert.Equal(t, "仅管理员可登录管理后台", resp.Message)
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

func newRegisterMultipartBody(email, password, code string, nickname *string, avatar []byte) (*bytes.Buffer, string) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("email", email)
	_ = writer.WriteField("password", password)
	_ = writer.WriteField("code", code)
	if nickname != nil {
		_ = writer.WriteField("nickname", *nickname)
	}
	if len(avatar) > 0 {
		part, _ := writer.CreateFormFile("avatar", "avatar.png")
		_, _ = part.Write(avatar)
	}
	_ = writer.Close()
	return body, writer.FormDataContentType()
}
