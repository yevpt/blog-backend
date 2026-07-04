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
	userservice "github.com/vpt/blog-backend/internal/service/user"
	"github.com/vpt/blog-backend/pkg/roles"
	"github.com/vpt/blog-backend/pkg/response"
)

type stubUserAdminService struct {
	grantUserID  uint
	revokeUserID uint
	resp         *dto.AdminUserRolesResp
	err          error
	listResp     *dto.AdminUserPageResp
	detailResp   *dto.AdminUserDetailResp
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

func TestUserAdminHandler_GrantVip_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{
		resp: &dto.AdminUserRolesResp{
			UserID: 42,
			Roles:  []string{roles.NormalRole, roles.VipRole},
		},
	}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop())

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
}

func TestUserAdminHandler_GrantVip_UserNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{err: userservice.ErrUserNotFound}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/users/99/roles/vip", nil)
	c.Params = gin.Params{{Key: "id", Value: "99"}}

	h.GrantVip(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUserAdminHandler_NormalizeAvatars_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserAdminService{}
	h := userhandler.NewUserAdminHandler(stub, zap.NewNop())

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
