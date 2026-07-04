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

func TestUserAdminHandler_GetOperationLogs_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{logsResp: &dto.AdminOperationLogPageResp{
		Total: 1, Pages: 1, Page: 1, PageSize: 10,
		List: []dto.AdminOperationLogItemResp{{ID: 1, OperatorID: 1, Action: "grant_vip"}},
	}}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop(), nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/users/7/operation-logs?page=1&page_size=10", nil)

	h.GetOperationLogs(c)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "grant_vip")
}

func TestUserAdminHandler_GetOperationLogs_BadQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop(), nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/users/7/operation-logs?page_size=999", nil)

	h.GetOperationLogs(c)
	// reqbind.Query 校验失败时统一响应仍是 HTTP 200，由 body 里的 code 字段区分业务失败
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"code":400`)
}

func TestUserAdminHandler_GetOperationLogs_BadPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop(), nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "not-a-number"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/users/not-a-number/operation-logs", nil)

	h.GetOperationLogs(c)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"code":400`)
}

func TestUserAdminHandler_GetOperationLogs_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{err: userservice.ErrUserNotFound}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop(), nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "99"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/users/99/operation-logs", nil)

	h.GetOperationLogs(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
