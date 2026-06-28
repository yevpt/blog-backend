package moderation

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

func requiredReviewerID(c *gin.Context) (uint64, bool) {
	claims := jwtpkg.GetClaims(c)
	if claims == nil || claims.UserId <= 0 {
		response.Unauthorized(c)
		return 0, false
	}
	return uint64(claims.UserId), true
}

func bindItemID(c *gin.Context) (uint64, bool) {
	id, ok := reqbind.PathUint(c, "id", "审核项 ID")
	return uint64(id), ok
}

func writeAdminResponse(c *gin.Context, data any, err error) {
	switch {
	case err == nil:
		response.Success(c, data)
	case errors.Is(err, moderationservice.ErrReviewConflict):
		response.Conflict(c, response.CodeModerationReviewConflict, moderationservice.PublicErrorMessage(err))
	case errors.Is(err, moderationservice.ErrAlreadyDeleted):
		response.Conflict(c, response.CodeContentAlreadyDeleted, moderationservice.PublicErrorMessage(err))
	case errors.Is(err, moderationservice.ErrItemNotFound), errors.Is(err, moderationservice.ErrSubjectNotFound):
		response.NotFound(c)
	case errors.Is(err, moderationservice.ErrInvalidRequest), errors.Is(err, moderationservice.ErrContentTooLong):
		response.Fail(c, response.CodeBadRequest, "审核参数不正确")
	default:
		response.ServerError(c)
	}
}

func moderationPageToDTO(page moderationservice.ReviewPage) dto.AdminModerationPageResp {
	items := make([]dto.AdminModerationItemResp, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, moderationItemToDTO(item))
	}
	return dto.AdminModerationPageResp{Total: page.Total, Page: page.Page, PageSize: page.PageSize, List: items}
}

func moderationItemToDTO(item moderationservice.ReviewItem) dto.AdminModerationItemResp {
	result := dto.AdminModerationItemResp{
		ItemID: item.ItemID,
		Subject: dto.AdminModerationSubjectResp{
			Type: string(item.Subject.Type), ID: item.Subject.ID, RootID: item.Subject.RootID,
			ParentID: cloneUint64(item.Subject.ParentID),
		},
		AuthorID: item.AuthorID, LockVersion: item.LockVersion,
		LifecycleState: string(item.LifecycleState), PublicState: string(item.PublicState),
		RevisionID: item.RevisionID, RevisionVersion: item.RevisionVersion,
		SubmittedContent: item.SubmittedContent, PublishedContent: item.PublishedContent,
		RiskLevel: string(item.RiskLevel), PolicyAction: string(item.PolicyAction),
		ReviewStatus: string(item.ReviewStatus), DecisionType: item.DecisionType,
		DecisionReason: item.DecisionReason, ReviewerID: item.ReviewerID, ReviewedAt: item.ReviewedAt,
		CreatedAt: item.CreatedAt, CanInteract: item.CanInteract,
	}
	if item.MomentOptions != nil {
		result.MomentOptions = &dto.AdminModerationMomentOptionsResp{
			Status: item.MomentOptions.Status, CommentStatus: item.MomentOptions.CommentStatus,
		}
	}
	return result
}

func cloneUint64(value *uint64) *uint64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
