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
