package user_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vpt/blog-backend/internal/dto"
	userhandler "github.com/vpt/blog-backend/internal/handler/user"
	"github.com/vpt/blog-backend/internal/middleware"
	userservice "github.com/vpt/blog-backend/internal/service/user"
	"github.com/vpt/blog-backend/pkg/roles"
	"github.com/vpt/blog-backend/pkg/response"
)

func TestUserAdminHandler_DisableAccount_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "9"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/users/9/disable", nil)
	middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 1, Username: "admin", Status: 1, Roles: []string{roles.AdminRole}})

	h.DisableAccount(c)

	require.Equal(t, http.StatusOK, w.Code)
	var body response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, 0, body.Code)
}

func TestUserAdminHandler_DisableAccount_Forbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{err: userservice.ErrLastAdminAccount}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "9"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/users/9/disable", nil)
	middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 1, Username: "admin", Status: 1, Roles: []string{roles.AdminRole}})

	h.DisableAccount(c)
	require.Equal(t, http.StatusOK, w.Code) // 统一响应包在 200 里，用 code 字段区分业务失败
	assert.Contains(t, w.Body.String(), "最后一个管理员")
}

func TestUserAdminHandler_DisableAccount_CannotDisableSelf(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{err: userservice.ErrCannotDisableSelf}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/users/1/disable", nil)
	middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 1, Username: "admin", Status: 1, Roles: []string{roles.AdminRole}})

	h.DisableAccount(c)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "不能禁用自己")
}

func TestUserAdminHandler_DisableAccount_UserNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{err: userservice.ErrUserNotFound}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "99"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/users/99/disable", nil)
	middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 1, Username: "admin", Status: 1, Roles: []string{roles.AdminRole}})

	h.DisableAccount(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUserAdminHandler_DisableAccount_Unauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "9"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/users/9/disable", nil)
	// 未设置 UserDetail，模拟未登录场景

	h.DisableAccount(c)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUserAdminHandler_DisableAccount_ServerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{err: assert.AnError}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "9"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/users/9/disable", nil)
	middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 1, Username: "admin", Status: 1, Roles: []string{roles.AdminRole}})

	h.DisableAccount(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUserAdminHandler_EnableAccount_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "9"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/users/9/enable", nil)

	h.EnableAccount(c)

	require.Equal(t, http.StatusOK, w.Code)
	var body response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, 0, body.Code)
}

func TestUserAdminHandler_EnableAccount_UserNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{err: userservice.ErrUserNotFound}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "99"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/users/99/enable", nil)

	h.EnableAccount(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
