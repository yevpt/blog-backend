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
	moderationhandler "github.com/vpt/blog-backend/internal/handler/moderation"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

type reviewServiceStub struct {
	approveCommand moderationservice.ReviewCommand
	rejectCommand  moderationservice.ReviewCommand
	approveResult  moderationservice.ReviewItem
	approveErr     error
	rejectErr      error
}

func (s *reviewServiceStub) List(context.Context, moderationservice.ListReviewCommand) (moderationservice.ReviewPage, error) {
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
