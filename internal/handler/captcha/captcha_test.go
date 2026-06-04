package captcha_test

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
	captchahandler "github.com/vpt/blog-backend/internal/handler/captcha"
	captchaservice "github.com/vpt/blog-backend/internal/service/captcha"
	"github.com/vpt/blog-backend/pkg/response"
)

type stubCaptchaService struct {
	challengeResp *dto.CaptchaChallengeResp
	challengeErr  error
	verifyResp    *dto.CaptchaVerifyResp
	verifyErr     error
	consumedToken string
	consumedIP    string
}

func (s *stubCaptchaService) GenerateRegistrationChallenge() (*dto.CaptchaChallengeResp, error) {
	return s.challengeResp, s.challengeErr
}

func (s *stubCaptchaService) VerifyRegistrationChallenge(req *dto.CaptchaVerifyReq, ip string) (*dto.CaptchaVerifyResp, error) {
	return s.verifyResp, s.verifyErr
}

func (s *stubCaptchaService) ConsumeRegistrationToken(token string, ip string) error {
	s.consumedToken = token
	s.consumedIP = ip
	return nil
}

func newTestRouter(svc captchaservice.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := captchahandler.NewCaptchaHandler(svc)
	r.POST("/captcha/register/challenge", h.GenerateRegistrationChallenge)
	r.POST("/captcha/register/verify", h.VerifyRegistrationChallenge)
	return r
}

func TestCaptchaHandler_GenerateRegistrationChallenge(t *testing.T) {
	stub := &stubCaptchaService{
		challengeResp: &dto.CaptchaChallengeResp{
			ChallengeID: "challenge-id",
			MasterImage: "data:image/jpeg;base64,master",
			TileImage:   "data:image/png;base64,tile",
			TileX:       10,
			TileY:       80,
			TileWidth:   60,
			TileHeight:  60,
			ImageWidth:  300,
			ImageHeight: 220,
		},
	}
	r := newTestRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/captcha/register/challenge", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestCaptchaHandler_VerifyRegistrationChallenge(t *testing.T) {
	stub := &stubCaptchaService{
		verifyResp: &dto.CaptchaVerifyResp{CaptchaToken: "captcha-token"},
	}
	r := newTestRouter(stub)
	body, _ := json.Marshal(map[string]any{
		"challenge_id": "challenge-id",
		"x":            160,
		"y":            80,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/captcha/register/verify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestCaptchaHandler_VerifyRegistrationChallengeInvalidBody(t *testing.T) {
	r := newTestRouter(&stubCaptchaService{})
	body, _ := json.Marshal(map[string]string{"challenge_id": "challenge-id"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/captcha/register/verify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, response.CodeBadRequest, resp.Code)
}

func TestCaptchaHandler_VerifyRegistrationChallengeInvalidCaptcha(t *testing.T) {
	stub := &stubCaptchaService{verifyErr: captchaservice.ErrInvalidCaptcha}
	r := newTestRouter(stub)
	body, _ := json.Marshal(map[string]any{
		"challenge_id": "challenge-id",
		"x":            12,
		"y":            80,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/captcha/register/verify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, response.CodeBadRequest, resp.Code)
}

func TestCaptchaHandler_GenerateRegistrationChallengeServerError(t *testing.T) {
	stub := &stubCaptchaService{challengeErr: errors.New("boom")}
	r := newTestRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/captcha/register/challenge", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
