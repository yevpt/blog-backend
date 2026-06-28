package moderation

import (
	"github.com/vpt/blog-backend/internal/dto"
)

// ProjectView 把内部审核视图转换为安全 DTO，并返回当前可展示正文。
func ProjectView(view View) (string, dto.ModerationView) {
	result := dto.ModerationView{
		PublicState:        string(view.PublicState),
		DisplayVersion:     string(view.DisplayVersion),
		HasPendingRevision: view.HasPendingRevision,
		PendingContent:     cloneString(view.PendingContent),
		CanInteract:        view.CanInteract,
	}
	if view.PendingRiskLevel != nil {
		value := string(*view.PendingRiskLevel)
		result.PendingRiskLevel = &value
	}
	if view.PendingReviewStatus != nil {
		value := string(*view.PendingReviewStatus)
		result.ReviewStatus = &value
	}
	return view.VisibleContent, result
}

// ProjectSubmitResult 把一次写入结果转换为作者可见的安全审核状态。
func ProjectSubmitResult(result SubmitResult) (string, dto.ModerationView) {
	displayVersion := DisplayNone
	switch {
	case result.Action == ActionPostReview && result.PublicState == PublicVisible:
		displayVersion = DisplayPending
	case result.Content != "" && result.PublicState == PublicVisible:
		displayVersion = DisplayLastApproved
	}
	pendingRisk := string(result.RiskLevel)
	reviewStatus := string(result.ReviewStatus)
	view := dto.ModerationView{
		Notice:             result.Message,
		PublicState:        string(result.PublicState),
		DisplayVersion:     string(displayVersion),
		HasPendingRevision: result.HasPendingRevision,
		PendingContent:     cloneString(result.PendingContent),
		CanInteract:        result.CanInteract,
	}
	if result.HasPendingRevision {
		view.PendingRiskLevel = &pendingRisk
		view.ReviewStatus = &reviewStatus
	}
	return result.Content, view
}
