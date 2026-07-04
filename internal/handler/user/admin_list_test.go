package user_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vpt/blog-backend/internal/dto"
	userhandler "github.com/vpt/blog-backend/internal/handler/user"
	userservice "github.com/vpt/blog-backend/internal/service/user"
)

func TestUserAdminHandler_ListAdmin_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{listResp: &dto.AdminUserPageResp{Total: 1, Page: 1, PageSize: 10}}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop(), nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/users?page=1&page_size=10", nil)

	h.ListAdmin(c)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestUserAdminHandler_ListAdmin_BadQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop(), nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/users?role=ROLE_BOGUS", nil)

	h.ListAdmin(c)
	// reqbind.Query 校验失败时统一响应仍是 HTTP 200，由 body 里的 code 字段区分业务失败
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"code":400`)
}

func TestUserAdminHandler_GetDetail_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{detailResp: &dto.AdminUserDetailResp{ID: 9, Username: "vpt"}}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop(), nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "9"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/users/9", nil)

	h.GetDetail(c)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestUserAdminHandler_GetDetail_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{err: userservice.ErrUserNotFound}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop(), nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "99"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/users/99", nil)

	h.GetDetail(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
