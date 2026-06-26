package user_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/dto"
	user "github.com/vpt/blog-backend/internal/handler/user"
	"github.com/vpt/blog-backend/internal/middleware"
	userservice "github.com/vpt/blog-backend/internal/service/user"
	"github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

type stubUserService struct {
	resp   *dto.UserDetailResp
	err    error
	userID uint

	emailTarget  string
	email        string
	code         string
	newPassword  string
	captchaToken string
	likedReq     dto.UserLikedContentListReq
	likedCount   int64
}

func (s *stubUserService) GetDetail(userID uint) (*dto.UserDetailResp, error) {
	s.userID = userID
	return s.resp, s.err
}

func (s *stubUserService) ListRecent(req *dto.UserListReq) (*dto.UserPageResp, error) {
	return nil, nil
}

func (s *stubUserService) ListAll(req *dto.UserListReq) (*dto.UserPageResp, error) {
	return nil, nil
}

func (s *stubUserService) Update(userID uint, req *dto.UserUpdateReq) (*dto.UserDetailResp, error) {
	return nil, nil
}

func (s *stubUserService) ChangeAvatar(userID uint, file *dto.UploadedImageFile) (*dto.UserDetailResp, error) {
	return nil, nil
}

func (s *stubUserService) GetPublicProfile(userID uint) (*dto.UserPublicProfileResp, error) {
	return nil, nil
}

func (s *stubUserService) ListLikedContent(userID uint, req dto.UserLikedContentListReq) (*dto.UserLikedContentPageResp, error) {
	s.userID = userID
	s.likedReq = req
	return &dto.UserLikedContentPageResp{
		Total:    1,
		Pages:    1,
		Page:     2,
		PageSize: 5,
		List: []dto.UserLikedContentItemResp{
			{
				ID:      9,
				Kind:    dto.UserLikedContentKindArticle,
				Filter:  dto.UserLikedContentFilterArticle,
				Content: dto.UserLikedContentObjectResp{ID: 3, Title: ptrString("文章")},
			},
		},
	}, nil
}

func (s *stubUserService) CountLikedContent(userID uint) (*dto.UserLikedContentCountResp, error) {
	s.userID = userID
	return &dto.UserLikedContentCountResp{Count: s.likedCount}, nil
}

func (s *stubUserService) UpdateProfile(userID uint, req *dto.UpdateProfileReq) (*dto.UserDetailResp, error) {
	return nil, nil
}

func (s *stubUserService) UpdateMeta(userID uint, req *dto.UpdateMetaReq) (*dto.UserDetailResp, error) {
	return nil, nil
}

func (s *stubUserService) UpdateSocialLink(userID uint, platform string, url *string) (*dto.UserDetailResp, error) {
	return nil, nil
}

func (s *stubUserService) UpdateUsername(userID uint, username string) error {
	return nil
}

func (s *stubUserService) UpdatePassword(userID uint, oldPwd, newPwd string) error {
	return nil
}

func (s *stubUserService) SetInitialPassword(userID uint, newPwd, code string) error {
	s.userID = userID
	s.newPassword = newPwd
	s.code = code
	return nil
}

func (s *stubUserService) SendEmailCode(userID uint, emailAddr, captchaToken, ip string) error {
	s.userID = userID
	s.email = emailAddr
	s.captchaToken = captchaToken
	return nil
}

func (s *stubUserService) UpdateEmail(userID uint, target, emailAddr, code string) error {
	s.userID = userID
	s.emailTarget = target
	s.email = emailAddr
	s.code = code
	return nil
}

func (s *stubUserService) UpdateEmailDisplay(userID uint, display string) error {
	return nil
}

// newUserRouter 构建测试路由，Auth 使用 nil cache（跳过缓存加载），
// 测试中通过 middleware.SetUserDetail 手动注入用户资料。
func newUserRouter(svc userservice.UserService, jwtManager *jwt.Manager, detail *dto.UserDetailResp) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := user.NewUserHandler(svc)
	authed := r.Group("/", middleware.Auth(jwtManager, nil))
	if detail != nil {
		// 在 Auth 之后通过中间件注入 UserDetail，模拟 userCache 已加载的状态
		authed.Use(func(c *gin.Context) {
			middleware.SetUserDetail(c, detail)
			c.Next()
		})
	}
	authed.GET("/users/me", h.GetDetail)
	authed.POST("/users/me/email/code", h.SendEmailCode)
	authed.PATCH("/users/me/email", h.UpdateEmail)
	authed.PATCH("/users/me/password/initial", h.SetInitialPassword)
	return r
}

func newPublicUserRouter(svc userservice.UserService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := user.NewUserHandler(svc)
	r.GET("/users/:id/likes", h.ListLikedContent)
	r.GET("/users/:id/likes/count", h.CountLikedContent)
	return r
}

func TestUserHandler_GetDetail_Success(t *testing.T) {
	jwtManager := jwt.NewManager("secret", 2, 168)
	emailAddr := "alice@example.com"
	detail := &dto.UserDetailResp{
		ID:       7,
		Username: "alice",
		Email:    &emailAddr,
		Roles:    []string{"ROLE_NORMAL"},
		Status:   1,
	}
	r := newUserRouter(&stubUserService{resp: detail}, jwtManager, detail)
	token, err := jwtManager.GenerateAccess(7)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int                `json:"code"`
		Data dto.UserDetailResp `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeOK, resp.Code)
	assert.Equal(t, "alice", resp.Data.Username)
}

func TestUserHandler_GetDetail_Unauthorized(t *testing.T) {
	jwtManager := jwt.NewManager("secret", 2, 168)
	r := newUserRouter(&stubUserService{}, jwtManager, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/users/me", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestUserHandler_GetDetail_NilDetail 验证 userCache 未返回资料时（nil detail）返回 401。
func TestUserHandler_GetDetail_NilDetail(t *testing.T) {
	jwtManager := jwt.NewManager("secret", 2, 168)
	// 传入 nil detail，模拟 userCache 加载失败或 Auth 中间件因 cache 为 nil 未写入 detail
	r := newUserRouter(&stubUserService{}, jwtManager, nil)
	token, err := jwtManager.GenerateAccess(9)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUserHandler_SendEmailCode_Success(t *testing.T) {
	jwtManager := jwt.NewManager("secret", 2, 168)
	detail := &dto.UserDetailResp{ID: 7, Username: "alice", Status: 1}
	svc := &stubUserService{}
	r := newUserRouter(svc, jwtManager, detail)
	token, err := jwtManager.GenerateAccess(7)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"email":"new@example.com","captcha_token":"captcha-token"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/users/me/email/code", body)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(7), svc.userID)
	assert.Equal(t, "new@example.com", svc.email)
	assert.Equal(t, "captcha-token", svc.captchaToken)
}

func TestUserHandler_UpdateEmail_Success(t *testing.T) {
	jwtManager := jwt.NewManager("secret", 2, 168)
	detail := &dto.UserDetailResp{ID: 7, Username: "alice", Status: 1}
	svc := &stubUserService{}
	r := newUserRouter(svc, jwtManager, detail)
	token, err := jwtManager.GenerateAccess(7)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"target":"main","email":"new@example.com","code":"123456"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/users/me/email", body)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "main", svc.emailTarget)
	assert.Equal(t, "new@example.com", svc.email)
	assert.Equal(t, "123456", svc.code)
}

func TestUserHandler_SetInitialPassword_Success(t *testing.T) {
	jwtManager := jwt.NewManager("secret", 2, 168)
	detail := &dto.UserDetailResp{ID: 7, Username: "alice", Status: 1}
	svc := &stubUserService{}
	r := newUserRouter(svc, jwtManager, detail)
	token, err := jwtManager.GenerateAccess(7)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"new_password":"new-password","code":"123456"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/users/me/password/initial", body)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "new-password", svc.newPassword)
	assert.Equal(t, "123456", svc.code)
}

func TestUserHandler_ListLikedContent_PublicBindsQuery(t *testing.T) {
	svc := &stubUserService{}
	r := newPublicUserRouter(svc)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/users/7/likes?page=2&page_size=5&type=comment", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(7), svc.userID)
	assert.Equal(t, 2, svc.likedReq.Page)
	assert.Equal(t, 5, svc.likedReq.PageSize)
	assert.Equal(t, dto.UserLikedContentFilterComment, svc.likedReq.Type)

	var resp struct {
		Code int                          `json:"code"`
		Data dto.UserLikedContentPageResp `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeOK, resp.Code)
	require.Len(t, resp.Data.List, 1)
	assert.Equal(t, dto.UserLikedContentKindArticle, resp.Data.List[0].Kind)
}

func TestUserHandler_CountLikedContent_Public(t *testing.T) {
	svc := &stubUserService{likedCount: 12}
	r := newPublicUserRouter(svc)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/users/7/likes/count", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(7), svc.userID)

	var resp struct {
		Code int                           `json:"code"`
		Data dto.UserLikedContentCountResp `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeOK, resp.Code)
	assert.Equal(t, int64(12), resp.Data.Count)
}

func ptrString(value string) *string {
	return &value
}
