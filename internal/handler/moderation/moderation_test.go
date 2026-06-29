package moderation_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderationhandler "github.com/vpt/blog-backend/internal/handler/moderation"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

type reviewServiceStub struct {
	approveCommand moderationservice.ReviewCommand
	rejectCommand  moderationservice.ReviewCommand
	listCommand    moderationservice.ListReviewCommand
	approveResult  moderationservice.ReviewItem
	approveErr     error
	rejectErr      error
}

type operationsServiceStub struct {
	updateControlCommand moderationservice.UpdateControlCommand
	hideItemCommand      moderationservice.EmergencyItemCommand
}

func (s *operationsServiceStub) GetControl(context.Context) (moderationservice.Control, error) {
	return moderationservice.Control{}, nil
}

func (s *operationsServiceStub) UpdateControl(_ context.Context, cmd moderationservice.UpdateControlCommand) (moderationservice.Control, error) {
	s.updateControlCommand = cmd
	return moderationservice.Control{LockVersion: cmd.ExpectedLockVersion + 1}, nil
}

func (s *operationsServiceStub) HideItem(_ context.Context, cmd moderationservice.EmergencyItemCommand) (moderationservice.EmergencyItemResult, error) {
	s.hideItemCommand = cmd
	return moderationservice.EmergencyItemResult{ItemID: cmd.ItemID, PublicState: moderationservice.PublicEmergencyHidden}, nil
}

func (s *operationsServiceStub) RestoreItem(context.Context, moderationservice.EmergencyItemCommand) (moderationservice.EmergencyItemResult, error) {
	return moderationservice.EmergencyItemResult{}, nil
}

func (s *operationsServiceStub) HideUserContent(context.Context, moderationservice.UserEmergencyBatchCommand) (moderationservice.EmergencyBatchResult, error) {
	return moderationservice.EmergencyBatchResult{}, nil
}

func (s *operationsServiceStub) RestoreUserContent(context.Context, moderationservice.UserEmergencyBatchCommand) (moderationservice.EmergencyBatchResult, error) {
	return moderationservice.EmergencyBatchResult{}, nil
}

func (s *operationsServiceStub) GetUserProfile(context.Context, uint64) (moderationservice.UserModerationProfile, error) {
	return moderationservice.UserModerationProfile{}, nil
}

func (s *operationsServiceStub) SetUserTrust(context.Context, moderationservice.SetTrustCommand) error {
	return nil
}

func (s *operationsServiceStub) SetUserSanction(context.Context, moderationservice.SetSanctionCommand) error {
	return nil
}

func (s *operationsServiceStub) ReleaseUserSanction(context.Context, uint64, uint64) error {
	return nil
}

func (s *reviewServiceStub) List(_ context.Context, cmd moderationservice.ListReviewCommand) (moderationservice.ReviewPage, error) {
	s.listCommand = cmd
	return moderationservice.ReviewPage{}, nil
}

func (s *reviewServiceStub) Get(context.Context, uint64) (moderationservice.ReviewItem, error) {
	return moderationservice.ReviewItem{}, nil
}

func (s *reviewServiceStub) Approve(_ context.Context, cmd moderationservice.ReviewCommand) (moderationservice.ReviewItem, error) {
	s.approveCommand = cmd
	return s.approveResult, s.approveErr
}

func (s *reviewServiceStub) Correct(context.Context, moderationservice.CorrectCommand) (moderationservice.ReviewItem, error) {
	return moderationservice.ReviewItem{}, nil
}

func (s *reviewServiceStub) Reject(_ context.Context, cmd moderationservice.ReviewCommand) (moderationservice.ReviewItem, error) {
	s.rejectCommand = cmd
	return moderationservice.ReviewItem{}, s.rejectErr
}

func TestAdminApproveUsesJWTReviewerAndReturnsUpdatedReview(t *testing.T) {
	stub := &reviewServiceStub{approveResult: moderationservice.ReviewItem{
		ItemID: 10, RevisionID: 20, ReviewStatus: moderationservice.ReviewApproved,
	}}
	handler := moderationhandler.NewAdminHandler(stub)
	recorder := serveReviewRequest(http.MethodPost, "/admin/moderation/items/10/approve",
		`{"revision_id":20,"lock_version":3}`, handler.Approve, true)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, uint64(10), stub.approveCommand.ItemID)
	assert.Equal(t, uint64(20), stub.approveCommand.RevisionID)
	assert.Equal(t, uint64(3), stub.approveCommand.ExpectedLockVersion)
	assert.Equal(t, uint64(1), stub.approveCommand.ReviewerID)
	assert.Contains(t, recorder.Body.String(), `"review_status":"approved"`)
}

func TestAdminReviewConflictUsesStable409Code(t *testing.T) {
	stub := &reviewServiceStub{rejectErr: moderationservice.ErrReviewConflict}
	handler := moderationhandler.NewAdminHandler(stub)
	recorder := serveReviewRequest(http.MethodPost, "/admin/moderation/items/10/reject",
		`{"revision_id":20,"lock_version":3,"reason":"不通过"}`, handler.Reject, true)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), response.CodeModerationReviewConflict)
}

func TestAdminReviewRequiresJWTClaims(t *testing.T) {
	stub := &reviewServiceStub{}
	handler := moderationhandler.NewAdminHandler(stub)
	recorder := serveReviewRequest(http.MethodPost, "/admin/moderation/items/10/approve",
		`{"revision_id":20,"lock_version":3}`, handler.Approve, false)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Zero(t, stub.approveCommand.ReviewerID)
}

func TestAdminReviewInvalidBodyReturnsBusiness400(t *testing.T) {
	stub := &reviewServiceStub{approveErr: errors.New("must not be called")}
	handler := moderationhandler.NewAdminHandler(stub)
	recorder := serveReviewRequest(http.MethodPost, "/admin/moderation/items/10/approve",
		`{"revision_id":0,"lock_version":3}`, handler.Approve, true)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"code":400`)
	assert.Zero(t, stub.approveCommand.ReviewerID)
}

func TestAdminUpdateControlUsesJWTActor(t *testing.T) {
	ops := &operationsServiceStub{}
	handler := moderationhandler.NewAdminHandler(&reviewServiceStub{}, ops)
	recorder := serveReviewRequest(http.MethodPatch, "/admin/moderation/control",
		`{"registration_mode":"closed","publishing_mode":"pre_review_all","reason":"维护","lock_version":3}`,
		handler.UpdateControl, true)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, uint64(1), ops.updateControlCommand.OperatorID)
	assert.Equal(t, uint64(3), ops.updateControlCommand.ExpectedLockVersion)
	assert.Equal(t, moderationservice.RegistrationClosed, ops.updateControlCommand.RegistrationMode)
}

func TestAdminEmergencyHideRequiresReasonAndUsesJWTActor(t *testing.T) {
	ops := &operationsServiceStub{}
	handler := moderationhandler.NewAdminHandler(&reviewServiceStub{}, ops)
	recorder := serveReviewRequest(http.MethodPost, "/admin/moderation/items/10/hide",
		`{"reason":"紧急下架"}`, handler.HideItem, true)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, uint64(1), ops.hideItemCommand.ActorID)
	assert.Equal(t, uint64(10), ops.hideItemCommand.ItemID)
	assert.Equal(t, "紧急下架", ops.hideItemCommand.Reason)
}

func serveReviewRequest(
	method string,
	path string,
	body string,
	action gin.HandlerFunc,
	withClaims bool,
) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: "10"}}
	if withClaims {
		jwtpkg.SetClaims(ctx, &jwtpkg.Claims{UserId: 1})
	}
	action(ctx)
	return recorder
}

func TestAdminListBindsPublicStateFilter(t *testing.T) {
	stub := &reviewServiceStub{}
	handler := moderationhandler.NewAdminHandler(stub)
	recorder := serveReviewRequest(http.MethodGet,
		"/admin/moderation/items?public_state=emergency_hidden&review_status=approved",
		"", handler.List, true)

	assert.Equal(t, http.StatusOK, recorder.Code)
	require.NotNil(t, stub.listCommand.ReviewStatus)
	assert.Equal(t, moderationservice.ReviewApproved, *stub.listCommand.ReviewStatus)
	require.NotNil(t, stub.listCommand.PublicState)
	assert.Equal(t, moderationservice.PublicEmergencyHidden, *stub.listCommand.PublicState)
}

func TestAdminListReviewStatusAllIncludesEveryStatus(t *testing.T) {
	stub := &reviewServiceStub{}
	handler := moderationhandler.NewAdminHandler(stub)
	recorder := serveReviewRequest(http.MethodGet,
		"/admin/moderation/items?review_status=all",
		"", handler.List, true)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.True(t, stub.listCommand.IncludeAllReviewStatuses)
	assert.Nil(t, stub.listCommand.ReviewStatus)
}
