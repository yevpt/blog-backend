package notification

import (
	"context"
	"encoding/json"

	notificationrepo "github.com/vpt/blog-backend/internal/repository/notification"
)

// SourceEngagementResolver 批量解析通知来源对象的点赞与回复数。
type SourceEngagementResolver interface {
	BatchSourceEngagement(ctx context.Context, viewerID uint, refs []notificationrepo.SourceEngagementRef) (map[notificationrepo.SourceEngagementKey]notificationrepo.SourceEngagement, error)
	BatchReplyCommentIDs(ctx context.Context, refs []notificationrepo.SourceEngagementRef) (map[uint]uint, error)
}

func sourceEngagementRefs(events []notificationrepo.InboxAggregate) []notificationrepo.SourceEngagementRef {
	refs := make([]notificationrepo.SourceEngagementRef, 0, len(events))
	for _, aggregate := range events {
		if !isEngagementSource(aggregate.Event.SourceType) {
			continue
		}
		refs = append(refs, notificationrepo.SourceEngagementRef{
			SourceType: aggregate.Event.SourceType,
			SourceID:   aggregate.Event.SourceID,
			RootType:   aggregate.Event.RootType,
		})
	}
	return refs
}

func isEngagementSource(sourceType string) bool {
	switch sourceType {
	case "comment", "reply", "guestbook":
		return true
	default:
		return false
	}
}

func metadataWithReplyCommentID(metadataJSON *string, commentID uint) *string {
	if commentID == 0 {
		return metadataJSON
	}
	meta := map[string]any{}
	if metadataJSON != nil && *metadataJSON != "" {
		_ = json.Unmarshal([]byte(*metadataJSON), &meta)
	}
	if existing, ok := meta["comment_id"].(float64); ok && uint(existing) == commentID {
		if metadataJSON != nil {
			return metadataJSON
		}
	}
	if existing, ok := meta["comment_id"].(uint); ok && existing == commentID {
		if metadataJSON != nil {
			return metadataJSON
		}
	}
	meta["comment_id"] = commentID
	encoded, err := json.Marshal(meta)
	if err != nil {
		return metadataJSON
	}
	value := string(encoded)
	return &value
}
