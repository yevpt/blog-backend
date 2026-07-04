package user_test

import (
	"context"
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
	"github.com/vpt/blog-backend/internal/service/adminlog"
	userservice "github.com/vpt/blog-backend/internal/service/user"
	"github.com/vpt/blog-backend/pkg/roles"
	"github.com/vpt/blog-backend/pkg/response"
)

// spyLogRecorder 是 adminlog.Recorder 的测试替身，记录每次 Record 调用的参数，
// 供测试断言"成功才记日志、失败不记日志"这条业务规则。
type spyLogRecorder struct {
	calls []spyLogRecorderCall
	err   error // 可选：配置 Record 的返回错误，用于验证 Record 失败不影响 HTTP 响应
}

type spyLogRecorderCall struct {
	operatorID   uint
	targetUserID uint
	action       adminlog.Action
	detail       map[string]any
}

func (s *spyLogRecorder) Record(_ context.Context, operatorID, targetUserID uint, action adminlog.Action, detail map[string]any) error {
	s.calls = append(s.calls, spyLogRecorderCall{
		operatorID:   operatorID,
		targetUserID: targetUserID,
		action:       action,
		detail:       detail,
	})
	return s.err
}

type stubUserAdminService struct {
	grantUserID  uint
	revokeUserID uint
	resp         *dto.AdminUserRolesResp
	err          error
	listResp     *dto.AdminUserPageResp
	detailResp   *dto.AdminUserDetailResp
	logsResp     *dto.AdminOperationLogPageResp
}

func (s *stubUserAdminService) GrantVip(targetUserID uint) (*dto.AdminUserRolesResp, error) {
	s.grantUserID = targetUserID
	return s.resp, s.err
}

func (s *stubUserAdminService) RevokeVip(targetUserID uint) (*dto.AdminUserRolesResp, error) {
	s.revokeUserID = targetUserID
	return s.resp, s.err
}

func (s *stubUserAdminService) NormalizeAvatars(ctx context.Context, req *dto.NormalizeAvatarsReq) (*dto.NormalizeAvatarsResp, error) {
	return &dto.NormalizeAvatarsResp{Scanned: 1, OK: 1}, s.err
}

func (s *stubUserAdminService) ClearUserAvatar(ctx context.Context, userID uint) (*dto.ClearUserAvatarResp, error) {
	return &dto.ClearUserAvatarResp{UserID: userID, OldKey: "avatar/user/old.jpg"}, s.err
}

func (s *stubUserAdminService) DisableAccount(operatorID, targetUserID uint) error {
	return s.err
}

func (s *stubUserAdminService) EnableAccount(targetUserID uint) error {
	return s.err
}

func (s *stubUserAdminService) ListAdmin(req *dto.AdminUserListReq) (*dto.AdminUserPageResp, error) {
	return s.listResp, s.err
}

func (s *stubUserAdminService) GetAdminDetail(userID uint) (*dto.AdminUserDetailResp, error) {
	return s.detailResp, s.err
}

func (s *stubUserAdminService) GetOperationLogs(targetUserID uint, page, pageSize int) (*dto.AdminOperationLogPageResp, error) {
	return s.logsResp, s.err
}

func TestUserAdminHandler_GrantVip_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{
		resp: &dto.AdminUserRolesResp{
			UserID: 42,
			Roles:  []string{roles.NormalRole, roles.VipRole},
		},
	}
	spy := &spyLogRecorder{}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop(), spy)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/users/42/roles/vip", nil)
	c.Params = gin.Params{{Key: "id", Value: "42"}}
	middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 1, Username: "admin", Status: 1, Roles: []string{roles.AdminRole}})

	h.GrantVip(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(42), stub.grantUserID)
	var body response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, 0, body.Code)

	require.Len(t, spy.calls, 1)
	assert.Equal(t, adminlog.ActionGrantVIP, spy.calls[0].action)
	assert.Equal(t, uint(42), spy.calls[0].targetUserID)
	assert.Equal(t, uint(1), spy.calls[0].operatorID)
}

func TestUserAdminHandler_GrantVip_UserNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{err: userservice.ErrUserNotFound}
	spy := &spyLogRecorder{}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop(), spy)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/users/99/roles/vip", nil)
	c.Params = gin.Params{{Key: "id", Value: "99"}}

	h.GrantVip(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Empty(t, spy.calls, "失败时不应记录操作日志")
}

func TestUserAdminHandler_RevokeVip_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{
		resp: &dto.AdminUserRolesResp{
			UserID: 42,
			Roles:  []string{roles.NormalRole},
		},
	}
	spy := &spyLogRecorder{}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop(), spy)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/admin/users/42/roles/vip", nil)
	c.Params = gin.Params{{Key: "id", Value: "42"}}
	middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 1, Username: "admin", Status: 1, Roles: []string{roles.AdminRole}})

	h.RevokeVip(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(42), stub.revokeUserID)

	require.Len(t, spy.calls, 1)
	assert.Equal(t, adminlog.ActionRevokeVIP, spy.calls[0].action)
	assert.Equal(t, uint(42), spy.calls[0].targetUserID)
}

func TestUserAdminHandler_RevokeVip_UserNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{err: userservice.ErrUserNotFound}
	spy := &spyLogRecorder{}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop(), spy)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/admin/users/99/roles/vip", nil)
	c.Params = gin.Params{{Key: "id", Value: "99"}}

	h.RevokeVip(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Empty(t, spy.calls, "失败时不应记录操作日志")
}

func TestUserAdminHandler_ClearUserAvatar_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{}
	spy := &spyLogRecorder{}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop(), spy)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/users/7/avatar/clear", nil)
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 1, Username: "admin", Status: 1, Roles: []string{roles.AdminRole}})

	h.ClearUserAvatar(c)

	assert.Equal(t, http.StatusOK, w.Code)

	require.Len(t, spy.calls, 1)
	assert.Equal(t, adminlog.ActionClearAvatar, spy.calls[0].action)
	assert.Equal(t, uint(7), spy.calls[0].targetUserID)
}

func TestUserAdminHandler_ClearUserAvatar_UserNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{err: userservice.ErrUserNotFound}
	spy := &spyLogRecorder{}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop(), spy)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/users/99/avatar/clear", nil)
	c.Params = gin.Params{{Key: "id", Value: "99"}}
	middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 1, Username: "admin", Status: 1, Roles: []string{roles.AdminRole}})

	h.ClearUserAvatar(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Empty(t, spy.calls, "失败时不应记录操作日志")
}

func TestUserAdminHandler_NormalizeAvatars_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop(), nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/users/avatars/normalize", nil)
	middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 1, Username: "admin", Status: 1, Roles: []string{roles.AdminRole}})

	h.NormalizeAvatars(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var body response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, 0, body.Code)
}
