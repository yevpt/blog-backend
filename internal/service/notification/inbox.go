package notification

import (
	"context"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	notificationrepo "github.com/vpt/blog-backend/internal/repository/notification"
	"github.com/vpt/blog-backend/pkg/storage"
)

// List 分页查询当前用户的站内通知，并把仓储聚合转换为 DTO。
func (s *inboxService) List(userID uint, req dto.NotificationListReq) (*dto.NotificationPageResp, error) {
	page := normalizePage(req.Page)
	pageSize := normalizePageSize(req.PageSize)

	result, err := s.repo.ListInbox(context.Background(), userID, req.UnreadOnly, page, pageSize)
	if err != nil {
		return nil, err
	}

	deleted, err := s.loadObjectDeletedStatus(context.Background(), result.Items)
	if err != nil {
		return nil, err
	}
	engagements, replyCommentIDs, err := s.loadSourceEngagements(context.Background(), userID, result.Items)
	if err != nil {
		return nil, err
	}

	// 逐条把收件箱 + 事件快照聚合映射为对外条目。
	items := make([]dto.NotificationItemResp, 0, len(result.Items))
	for _, aggregate := range result.Items {
		items = append(items, inboxAggregateToDTO(aggregate, deleted, engagements, replyCommentIDs, s.resolver))
	}
	return &dto.NotificationPageResp{
		Total:    result.Total,
		Page:     page,
		PageSize: pageSize,
		List:     items,
	}, nil
}

// UnreadCount 返回当前用户的未读通知数量。
func (s *inboxService) UnreadCount(userID uint) (*dto.NotificationUnreadCountResp, error) {
	count, err := s.repo.CountUnread(context.Background(), userID)
	if err != nil {
		return nil, err
	}
	return &dto.NotificationUnreadCountResp{Count: count}, nil
}

// MarkRead 将当前用户名下的单条通知置为已读；非本人或不存在时返回 ErrNotificationNotFound。
func (s *inboxService) MarkRead(userID uint, id uint) error {
	affected, err := s.repo.MarkInboxRead(context.Background(), userID, id)
	if err != nil {
		return err
	}
	// 受影响行为 0 说明通知不存在或不属于该用户，按归属校验失败处理。
	if affected == 0 {
		return ErrNotificationNotFound
	}
	return nil
}

// MarkAllRead 批量已读；ids 为空表示把当前用户全部未读置为已读。
func (s *inboxService) MarkAllRead(userID uint, ids []uint) (*dto.NotificationReadResp, error) {
	affected, err := s.repo.MarkAllInboxRead(context.Background(), userID, ids)
	if err != nil {
		return nil, err
	}
	return &dto.NotificationReadResp{Updated: affected}, nil
}

// Delete 软删除当前用户名下的单条通知；非本人或不存在时返回 ErrNotificationNotFound。
func (s *inboxService) Delete(userID uint, id uint) error {
	affected, err := s.repo.DeleteInbox(context.Background(), userID, id)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotificationNotFound
	}
	return nil
}

// inboxAggregateToDTO 把收件箱聚合映射为对外通知条目。
func inboxAggregateToDTO(
	aggregate notificationrepo.InboxAggregate,
	deleted map[notificationrepo.ObjectDeletedKey]bool,
	engagements map[notificationrepo.SourceEngagementKey]notificationrepo.SourceEngagement,
	replyCommentIDs map[uint]uint,
	resolver storage.ObjectURLResolver,
) dto.NotificationItemResp {
	metadata := aggregate.Event.MetadataJSON
	if aggregate.Event.SourceType == "reply" {
		metadata = metadataWithReplyCommentID(metadata, replyCommentIDs[aggregate.Event.SourceID])
	}

	resp := dto.NotificationItemResp{
		ID:             aggregate.Inbox.ID,
		EventID:        aggregate.Inbox.EventID,
		Type:           aggregate.Event.Type,
		Title:          aggregate.Event.Title,
		ContentExcerpt: aggregate.Event.ContentExcerpt,
		IsRead:         aggregate.Inbox.IsRead,
		ReadAt:         aggregate.Inbox.ReadAt,
		CreatedAt:      aggregate.Inbox.CreatedAt,
		ActorUserID:    aggregate.Event.ActorUserID,
		ActorUser:      actorUserToDTO(aggregate.ActorUser, resolver),
		SourceType:     aggregate.Event.SourceType,
		SourceID:       aggregate.Event.SourceID,
		RootType:       aggregate.Event.RootType,
		RootID:         aggregate.Event.RootID,
		SourceDeleted:  deleted[sourceDeletedKey(aggregate.Event)],
		RootDeleted:    deleted[rootDeletedKey(aggregate.Event)],
		Metadata:       metadata,
	}
	if isEngagementSource(aggregate.Event.SourceType) {
		if engagement, ok := engagements[notificationrepo.SourceEngagementKey{
			SourceType: aggregate.Event.SourceType,
			SourceID:   aggregate.Event.SourceID,
		}]; ok {
			likeCount := engagement.LikeCount
			isLiked := engagement.IsLiked
			replyCount := engagement.ReplyCount
			resp.LikeCount = &likeCount
			resp.IsLiked = &isLiked
			resp.ReplyCount = &replyCount
		}
	}
	return resp
}

func (s *inboxService) loadSourceEngagements(ctx context.Context, userID uint, items []notificationrepo.InboxAggregate) (
	map[notificationrepo.SourceEngagementKey]notificationrepo.SourceEngagement,
	map[uint]uint,
	error,
) {
	if s.engagement == nil {
		return nil, nil, nil
	}
	refs := sourceEngagementRefs(items)
	if len(refs) == 0 {
		return map[notificationrepo.SourceEngagementKey]notificationrepo.SourceEngagement{}, map[uint]uint{}, nil
	}
	engagements, err := s.engagement.BatchSourceEngagement(ctx, userID, refs)
	if err != nil {
		return nil, nil, err
	}
	replyRefs := make([]notificationrepo.SourceEngagementRef, 0)
	for _, aggregate := range items {
		if aggregate.Event.SourceType != "reply" {
			continue
		}
		replyRefs = append(replyRefs, notificationrepo.SourceEngagementRef{
			SourceType: aggregate.Event.SourceType,
			SourceID:   aggregate.Event.SourceID,
			RootType:   aggregate.Event.RootType,
		})
	}
	replyCommentIDs, err := s.engagement.BatchReplyCommentIDs(ctx, replyRefs)
	if err != nil {
		return nil, nil, err
	}
	return engagements, replyCommentIDs, nil
}

// actorUserToDTO 把操作人 model 映射为对外摘要，解析头像 URL。
// user 为 nil（系统通知）时返回 nil，保持 omitempty。
func actorUserToDTO(user *model.User, resolver storage.ObjectURLResolver) *dto.NotificationActorUserResp {
	if user == nil {
		return nil
	}
	return &dto.NotificationActorUserResp{
		ID:        user.ID,
		Nickname:  user.Nickname,
		AvatarUrl: storage.ResolvePtrURL(resolver, user.AvatarUrl),
		Site:      user.Site,
		Mark:      user.Mark,
	}
}

// normalizePage 规整页码，最小为 1。
func normalizePage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}

// normalizePageSize 规整每页数量，默认 10，最大 50。
func normalizePageSize(pageSize int) int {
	if pageSize < 1 {
		return 10
	}
	if pageSize > 50 {
		return 50
	}
	return pageSize
}
