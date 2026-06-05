package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler"
	"github.com/vpt/blog-backend/internal/middleware"
	"github.com/vpt/blog-backend/internal/service"
	"github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

type stubUserService struct {
	resp   *dto.UserDetailResp
	err    error
	userID uint
}

func (s *stubUserService) GetDetail(userID uint) (*dto.UserDetailResp, error) {
	s.userID = userID
	return s.resp, s.err
}

// newUserRouter 构建测试路由，Auth 使用 nil cache（跳过缓存加载），
// 测试中通过 middleware.SetUserDetail 手动注入用户资料。
func newUserRouter(svc service.UserService, jwtManager *jwt.Manager, detail *dto.UserDetailResp) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewUserHandler(svc)
	authed := r.Group("/", middleware.Auth(jwtManager, nil))
	if detail != nil {
		// 在 Auth 之后通过中间件注入 UserDetail，模拟 userCache 已加载的状态
		authed.Use(func(c *gin.Context) {
			middleware.SetUserDetail(c, detail)
			c.Next()
		})
	}
	authed.GET("/users/me", h.GetDetail)
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
	r := newUserRouter(&stubUserService{}, jwtManager, detail)
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
