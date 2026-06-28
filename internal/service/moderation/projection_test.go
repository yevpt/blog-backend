package moderation_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/dto"
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/internal/service/moderation"
)

func TestProjectViewBuildsSafeDTOWithoutRuleIDs(t *testing.T) {
	content := "待审编辑正文 ![图](comments/new.jpg)"
	pendingProjected := "待审编辑正文 ![图](moderation/previews/new.jpg)"
	risk := moderationrepo.RiskMedium
	status := moderationrepo.ReviewPending
	view := moderationrepo.View{
		PublicState: moderationrepo.PublicVisible, DisplayVersion: moderationrepo.DisplayLastApproved,
		VisibleContent: "旧正文 ![图](comments/old.jpg)", HasPendingRevision: true,
		PendingContent: &content, PendingRiskLevel: &risk, PendingReviewStatus: &status,
		PendingRuleMatchIDs: []uint64{3, 4}, CanInteract: false,
		VisibleImages: []moderationrepo.ImageView{{SourceObjectKey: "comments/old.jpg", DisplayObjectKey: "moderation/previews/old.jpg"}},
		PendingImages: []moderationrepo.ImageView{{SourceObjectKey: "comments/new.jpg", DisplayObjectKey: "moderation/previews/new.jpg"}},
	}

	gotContent, got := moderation.ProjectView(view)

	assert.Equal(t, "旧正文 ![图](moderation/previews/old.jpg)", gotContent)
	assert.Equal(t, dto.ModerationView{
		PublicState: "visible", DisplayVersion: "last_approved", HasPendingRevision: true,
		PendingRiskLevel: stringPointer("medium"), ReviewStatus: stringPointer("pending"),
		PendingContent: &pendingProjected, CanInteract: false,
	}, got)
	require.NotContains(t, anyJSONKeys(got), "rule_match_ids")
}

func stringPointer(value string) *string { return &value }

func anyJSONKeys(value any) map[string]any {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		panic(err)
	}
	return result
}
