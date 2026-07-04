package moderation_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderationhandler "github.com/vpt/blog-backend/internal/handler/moderation"
	"github.com/vpt/blog-backend/internal/service/adminlog"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
)

// sanctionOpsStub 复用 operationsServiceStub 的其余方法，只重写和处罚/信任等级相关的三个方法，
// 用来模拟成功/失败两种返回，从而验证 setSanction/ReleaseUser/UpdateUserProfile 的日志门控。
type sanctionOpsStub struct {
	operationsServiceStub

	setSanctionErr        error
	setSanctionCommand    moderationservice.SetSanctionCommand
	releaseSanctionErr    error
	releaseSanctionUserID uint64
	releaseSanctionActor  uint64
	setTrustErr           error
	setTrustCommand       moderationservice.SetTrustCommand
}

func (s *sanctionOpsStub) SetUserSanction(_ context.Context, cmd moderationservice.SetSanctionCommand) error {
	s.setSanctionCommand = cmd
	return s.setSanctionErr
}

func (s *sanctionOpsStub) ReleaseUserSanction(_ context.Context, userID, actorID uint64) error {
	s.releaseSanctionUserID = userID
	s.releaseSanctionActor = actorID
	return s.releaseSanctionErr
}

func (s *sanctionOpsStub) SetUserTrust(_ context.Context, cmd moderationservice.SetTrustCommand) error {
	s.setTrustCommand = cmd
	return s.setTrustErr
}

// recorderSpy 是记录每次 Record 调用的 spy 实现，用来验证"仅成功才记录"的门控逻辑。
type recorderSpy struct {
	mu    sync.Mutex
	calls []recorderCall
}

type recorderCall struct {
	operatorID   uint
	targetUserID uint
	action       adminlog.Action
	detail       map[string]any
}

func (r *recorderSpy) Record(_ context.Context, operatorID, targetUserID uint, action adminlog.Action, detail map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recorderCall{operatorID: operatorID, targetUserID: targetUserID, action: action, detail: detail})
	return nil
}

func serveOperationsRequest(
	method string,
	path string,
	body string,
	action gin.HandlerFunc,
) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: "10"}}
	jwtpkg.SetClaims(ctx, &jwtpkg.Claims{UserId: 1})
	action(ctx)
	return recorder
}

func TestMuteUserRecordsLogOnlyOnSuccess(t *testing.T) {
	ops := &sanctionOpsStub{}
	spy := &recorderSpy{}
	handler := moderationhandler.NewAdminHandler(&reviewServiceStub{}, nil, ops)
	handler.SetOperationLogRecorder(spy)

	recorder := serveOperationsRequest(http.MethodPost, "/admin/moderation/users/10/mute",
		`{"reason":"违规发言"}`, handler.MuteUser)

	assert.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, spy.calls, 1)
	assert.Equal(t, adminlog.ActionMute, spy.calls[0].action)
	assert.Equal(t, uint(10), spy.calls[0].targetUserID)
	assert.Equal(t, uint(1), spy.calls[0].operatorID)
}

func TestMuteUserDoesNotRecordLogOnFailure(t *testing.T) {
	ops := &sanctionOpsStub{setSanctionErr: errors.New("boom")}
	spy := &recorderSpy{}
	handler := moderationhandler.NewAdminHandler(&reviewServiceStub{}, nil, ops)
	handler.SetOperationLogRecorder(spy)

	serveOperationsRequest(http.MethodPost, "/admin/moderation/users/10/mute", `{"reason":"违规发言"}`, handler.MuteUser)

	assert.Empty(t, spy.calls)
}

func TestBanUserRecordsLogOnlyOnSuccess(t *testing.T) {
	ops := &sanctionOpsStub{}
	spy := &recorderSpy{}
	handler := moderationhandler.NewAdminHandler(&reviewServiceStub{}, nil, ops)
	handler.SetOperationLogRecorder(spy)

	recorder := serveOperationsRequest(http.MethodPost, "/admin/moderation/users/10/ban",
		`{"reason":"严重违规"}`, handler.BanUser)

	assert.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, spy.calls, 1)
	assert.Equal(t, adminlog.ActionBan, spy.calls[0].action)
	assert.Equal(t, uint(10), spy.calls[0].targetUserID)
}

func TestBanUserDoesNotRecordLogOnFailure(t *testing.T) {
	ops := &sanctionOpsStub{setSanctionErr: errors.New("boom")}
	spy := &recorderSpy{}
	handler := moderationhandler.NewAdminHandler(&reviewServiceStub{}, nil, ops)
	handler.SetOperationLogRecorder(spy)

	serveOperationsRequest(http.MethodPost, "/admin/moderation/users/10/ban", `{"reason":"严重违规"}`, handler.BanUser)

	assert.Empty(t, spy.calls)
}

func TestReleaseUserRecordsLogOnlyOnSuccess(t *testing.T) {
	ops := &sanctionOpsStub{}
	spy := &recorderSpy{}
	handler := moderationhandler.NewAdminHandler(&reviewServiceStub{}, nil, ops)
	handler.SetOperationLogRecorder(spy)

	recorder := serveOperationsRequest(http.MethodPost, "/admin/moderation/users/10/release", "", handler.ReleaseUser)

	assert.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, spy.calls, 1)
	assert.Equal(t, adminlog.ActionRelease, spy.calls[0].action)
	assert.Equal(t, uint(10), spy.calls[0].targetUserID)
	assert.Equal(t, uint(1), spy.calls[0].operatorID)
}

func TestReleaseUserDoesNotRecordLogOnFailure(t *testing.T) {
	ops := &sanctionOpsStub{releaseSanctionErr: errors.New("boom")}
	spy := &recorderSpy{}
	handler := moderationhandler.NewAdminHandler(&reviewServiceStub{}, nil, ops)
	handler.SetOperationLogRecorder(spy)

	serveOperationsRequest(http.MethodPost, "/admin/moderation/users/10/release", "", handler.ReleaseUser)

	assert.Empty(t, spy.calls)
}

func TestUpdateUserProfileRecordsLogOnlyOnSuccess(t *testing.T) {
	ops := &sanctionOpsStub{}
	spy := &recorderSpy{}
	handler := moderationhandler.NewAdminHandler(&reviewServiceStub{}, nil, ops)
	handler.SetOperationLogRecorder(spy)

	recorder := serveOperationsRequest(http.MethodPatch, "/admin/moderation/users/10/profile",
		`{"trust_level":"trusted","manual_locked":true}`, handler.UpdateUserProfile)

	assert.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, spy.calls, 1)
	assert.Equal(t, adminlog.ActionUpdateTrustLevel, spy.calls[0].action)
	assert.Equal(t, uint(10), spy.calls[0].targetUserID)
	assert.Equal(t, uint(1), spy.calls[0].operatorID)
	require.NotNil(t, spy.calls[0].detail)
	assert.Equal(t, "trusted", spy.calls[0].detail["trust_level"])
}

func TestUpdateUserProfileDoesNotRecordLogOnFailure(t *testing.T) {
	ops := &sanctionOpsStub{setTrustErr: errors.New("boom")}
	spy := &recorderSpy{}
	handler := moderationhandler.NewAdminHandler(&reviewServiceStub{}, nil, ops)
	handler.SetOperationLogRecorder(spy)

	serveOperationsRequest(http.MethodPatch, "/admin/moderation/users/10/profile",
		`{"trust_level":"trusted","manual_locked":true}`, handler.UpdateUserProfile)

	assert.Empty(t, spy.calls)
}

func TestSetSanctionDoesNotRecordLogWhenRecorderNotConfigured(t *testing.T) {
	ops := &sanctionOpsStub{}
	handler := moderationhandler.NewAdminHandler(&reviewServiceStub{}, nil, ops)

	recorder := serveOperationsRequest(http.MethodPost, "/admin/moderation/users/10/mute",
		`{"reason":"违规发言"}`, handler.MuteUser)

	assert.Equal(t, http.StatusOK, recorder.Code)
}
