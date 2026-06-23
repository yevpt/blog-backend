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

	// 按根对象去重解析展示标题，供前端直接显示「来自哪篇文章/哪条碎语」。
	labels, err := s.rootLabels(context.Background(), result.Items)
	if err != nil {
		return nil, err
	}
	// 按根对象去重解析文章正文摘录，仅对无评论/回复内容的通知填充。
	excerpts, err := s.rootExcerpts(context.Background(), result.Items)
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
		items = append(items, inboxAggregateToDTO(aggregate, labels, excerpts, engagements, replyCommentIDs, s.resolver))
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
	labels map[rootKey]string,
	excerpts map[rootKey]string,
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
		Metadata:       metadata,
	}
	if label, ok := labels[rootKey{rootType: aggregate.Event.RootType, rootID: aggregate.Event.RootID}]; ok && label != "" {
		resp.RootTitle = &label
	}
	if aggregate.Event.RootType == "article" {
		if excerpt, ok := excerpts[rootKey{rootType: aggregate.Event.RootType, rootID: aggregate.Event.RootID}]; ok && excerpt != "" {
			resp.RootExcerpt = &excerpt
		}
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

// rootKey 是根对象的复合去重键。
type rootKey struct {
	rootType string
	rootID   uint
}

// rootLabels 批量解析当前页事件根对象的展示标题，按 (rootType, rootID) 去重。
// roots 为 nil 时返回空 map，调用方据此跳过 RootTitle 填充，保持旧行为。
// 仅解析 article/moment 两类根对象，其余类型（留言板等）无展示标题。
func (s *inboxService) rootLabels(ctx context.Context, items []notificationrepo.InboxAggregate) (map[rootKey]string, error) {
	if s.roots == nil {
		return nil, nil
	}
	out := make(map[rootKey]string, len(items))
	for _, aggregate := range items {
		if aggregate.Event.RootID == 0 {
			continue
		}
		switch aggregate.Event.RootType {
		case "article", "moment":
		default:
			continue
		}
		key := rootKey{rootType: aggregate.Event.RootType, rootID: aggregate.Event.RootID}
		if _, exists := out[key]; exists {
			continue
		}
		label, err := s.roots.RootSnapshotOf(ctx, aggregate.Event.RootType, aggregate.Event.RootID)
		if err != nil {
			return nil, err
		}
		out[key] = label
	}
	return out, nil
}

// rootExcerpts 批量解析当前页文章根对象的正文摘录，按 rootID 去重。
// content_excerpt 为评论/回复正文，与文章摘录互不重复，故评论类事件同样需要填充。
// roots 为 nil 时返回空 map。
func (s *inboxService) rootExcerpts(ctx context.Context, items []notificationrepo.InboxAggregate) (map[rootKey]string, error) {
	if s.roots == nil {
		return nil, nil
	}
	out := make(map[rootKey]string, len(items))
	for _, aggregate := range items {
		if aggregate.Event.RootID == 0 || aggregate.Event.RootType != "article" {
			continue
		}
		key := rootKey{rootType: aggregate.Event.RootType, rootID: aggregate.Event.RootID}
		if _, exists := out[key]; exists {
			continue
		}
		excerpt, err := s.roots.RootExcerptOf(ctx, aggregate.Event.RootType, aggregate.Event.RootID)
		if err != nil {
			return nil, err
		}
		out[key] = excerpt
	}
	return out, nil
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
