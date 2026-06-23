package notification_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	notificationrepo "github.com/vpt/blog-backend/internal/repository/notification"
	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
)

// fakeEventRepo 记录 CreateEvent 的入参，供 publisher 断言快照内容。
type fakeEventRepo struct {
	created     *model.NotificationEvent
	createErr   error
	createCalls int
}

func (f *fakeEventRepo) CreateEvent(_ context.Context, event *model.NotificationEvent) error {
	f.createCalls++
	f.created = event
	return f.createErr
}

func (f *fakeEventRepo) LeasePendingEvents(context.Context, string, int, int) ([]model.NotificationEvent, error) {
	return nil, nil
}
func (f *fakeEventRepo) MarkEventDone(context.Context, uint) error { return nil }
func (f *fakeEventRepo) MarkEventRetry(context.Context, uint, time.Time, string) error {
	return nil
}
func (f *fakeEventRepo) GetEventsByIDs(context.Context, []uint) (map[uint]model.NotificationEvent, error) {
	return nil, nil
}

// fakeInboxRepo 用可配置返回值驱动 InboxService 用例。
type fakeInboxRepo struct {
	listResp        *notificationrepo.InboxPage
	listErr         error
	listRecipientID uint
	listUnreadOnly  bool
	listPage        int
	listPageSize    int

	unreadCount int64

	markReadAffected    int64
	markAllReadAffected int64
	markAllReadIDs      []uint
	deleteAffected      int64
}

func (f *fakeInboxRepo) CreateInbox(context.Context, *model.NotificationInbox) (bool, error) {
	return true, nil
}

func (f *fakeInboxRepo) ListInbox(_ context.Context, recipientID uint, unreadOnly bool, page int, pageSize int) (*notificationrepo.InboxPage, error) {
	f.listRecipientID = recipientID
	f.listUnreadOnly = unreadOnly
	f.listPage = page
	f.listPageSize = pageSize
	return f.listResp, f.listErr
}

func (f *fakeInboxRepo) CountUnread(context.Context, uint) (int64, error) {
	return f.unreadCount, nil
}

func (f *fakeInboxRepo) MarkInboxRead(context.Context, uint, uint) (int64, error) {
	return f.markReadAffected, nil
}

func (f *fakeInboxRepo) MarkAllInboxRead(_ context.Context, _ uint, ids []uint) (int64, error) {
	f.markAllReadIDs = ids
	return f.markAllReadAffected, nil
}

func (f *fakeInboxRepo) DeleteInbox(context.Context, uint, uint) (int64, error) {
	return f.deleteAffected, nil
}

// publisher 应去除标题/摘要首尾空白并按列宽截断后落库。
func TestPublisher_TrimsAndSnapshotsContentExcerpt(t *testing.T) {
	repo := &fakeEventRepo{}
	publisher := notificationservice.NewPublisher(repo)

	longExcerpt := strings.Repeat("字", 600)
	event, err := publisher.Publish(context.Background(), notificationservice.PublishEvent{
		Type:           notificationservice.EventTypeCommentCreated,
		SourceType:     "comment",
		SourceID:       99,
		RootType:       "moment",
		RootID:         12,
		Title:          "  有人评论了你  ",
		ContentExcerpt: "  " + longExcerpt + "  ",
	})

	require.NoError(t, err)
	require.NotNil(t, event)
	assert.Equal(t, 1, repo.createCalls)
	// 标题去空白。
	assert.Equal(t, "有人评论了你", repo.created.Title)
	// 摘要去空白后按 500 rune 截断。
	assert.Equal(t, 500, len([]rune(repo.created.ContentExcerpt)))
	assert.False(t, strings.HasPrefix(repo.created.ContentExcerpt, " "))
	// 默认进入待分发状态。
	assert.Equal(t, notificationrepo.EventStatusPending, repo.created.DispatchStatus)
}

func TestBuildSourceRootMetadata_TrimsAndTruncatesSnapshots(t *testing.T) {
	longTitle := strings.Repeat("题", 150)
	longExcerpt := strings.Repeat("摘", 600)
	metadata := notificationservice.BuildSourceRootMetadata(
		notificationservice.NotificationSnapshot{Type: "article", ID: 1, Title: " " + longTitle + " ", Excerpt: " " + longExcerpt + " "},
		&notificationservice.NotificationSnapshot{Type: "article", ID: 1, Title: " " + longTitle + " ", Excerpt: " " + longExcerpt + " "},
	)

	require.NotNil(t, metadata)
	var decoded struct {
		SourceSnapshot notificationservice.NotificationSnapshot `json:"source_snapshot"`
		RootSnapshot   notificationservice.NotificationSnapshot `json:"root_snapshot"`
	}
	require.NoError(t, json.Unmarshal([]byte(*metadata), &decoded))
	assert.Equal(t, 120, len([]rune(decoded.SourceSnapshot.Title)))
	assert.Equal(t, 500, len([]rune(decoded.SourceSnapshot.Excerpt)))
	assert.False(t, strings.HasPrefix(decoded.SourceSnapshot.Title, " "))
	assert.Equal(t, 120, len([]rune(decoded.RootSnapshot.Title)))
	assert.Equal(t, 500, len([]rune(decoded.RootSnapshot.Excerpt)))
}

func TestPublisher_AllowsReplyLikedEvent(t *testing.T) {
	repo := &fakeEventRepo{}
	publisher := notificationservice.NewPublisher(repo)

	event, err := publisher.Publish(context.Background(), notificationservice.PublishEvent{
		Type:       notificationservice.EventTypeReplyLiked,
		SourceType: "reply",
		SourceID:   12,
		RootType:   "guestbook",
		RootID:     12,
	})

	require.NoError(t, err)
	require.NotNil(t, event)
	assert.Equal(t, notificationservice.EventTypeReplyLiked, repo.created.Type)
}

// 非法事件类型应被拒绝，且不触达仓储。
func TestPublisher_RejectsInvalidEventType(t *testing.T) {
	repo := &fakeEventRepo{}
	publisher := notificationservice.NewPublisher(repo)

	event, err := publisher.Publish(context.Background(), notificationservice.PublishEvent{
		Type:       "not_a_real_type",
		SourceType: "comment",
	})

	require.ErrorIs(t, err, notificationservice.ErrInvalidEventType)
	assert.Nil(t, event)
	assert.Equal(t, 0, repo.createCalls)
}

// 收件箱列表应把仓储聚合映射为 DTO。
func TestInboxService_List_MapsAggregateToDTO(t *testing.T) {
	actorID := uint(2)
	actorNickname := "VPT"
	readAt := time.Now()
	repo := &fakeInboxRepo{
		listResp: &notificationrepo.InboxPage{
			Total: 1,
			Items: []notificationrepo.InboxAggregate{
				{
					Inbox: model.NotificationInbox{
						Base:    model.Base{ID: 7},
						EventID: 10,
						IsRead:  true,
						ReadAt:  &readAt,
					},
					Event: model.NotificationEvent{
						Base:           model.Base{ID: 10},
						Type:           notificationservice.EventTypeCommentCreated,
						ActorUserID:    &actorID,
						SourceType:     "comment",
						SourceID:       99,
						RootType:       "moment",
						RootID:         12,
						Title:          "有人评论了你",
						ContentExcerpt: "写得真好",
					},
					ActorUser: &model.User{
						Base:     model.Base{ID: 2},
						Username: "vpt",
						Nickname: &actorNickname,
					},
				},
			},
		},
	}
	service := notificationservice.NewInboxService(repo, nil, nil, nil)

	resp, err := service.List(3, dto.NotificationListReq{Page: 2, PageSize: 5, UnreadOnly: true})

	require.NoError(t, err)
	require.NotNil(t, resp)
	// 透传过滤条件与分页。
	assert.Equal(t, uint(3), repo.listRecipientID)
	assert.True(t, repo.listUnreadOnly)
	assert.Equal(t, 2, repo.listPage)
	assert.Equal(t, 5, repo.listPageSize)
	// 聚合映射。
	require.Len(t, resp.List, 1)
	item := resp.List[0]
	assert.Equal(t, uint(7), item.ID)
	assert.Equal(t, uint(10), item.EventID)
	assert.Equal(t, notificationservice.EventTypeCommentCreated, item.Type)
	assert.Equal(t, "有人评论了你", item.Title)
	assert.Equal(t, "写得真好", item.ContentExcerpt)
	assert.True(t, item.IsRead)
	require.NotNil(t, item.ActorUserID)
	assert.Equal(t, uint(2), *item.ActorUserID)
	// 操作人摘要映射。
	require.NotNil(t, item.ActorUser)
	assert.Equal(t, uint(2), item.ActorUser.ID)
	require.NotNil(t, item.ActorUser.Nickname)
	assert.Equal(t, "VPT", *item.ActorUser.Nickname)
	assert.Equal(t, int64(1), resp.Total)
}

// 系统通知 ActorUserID 为空时，ActorUser 应为 nil，保持 omitempty。
func TestInboxService_List_SystemNoticeHasNilActorUser(t *testing.T) {
	repo := &fakeInboxRepo{
		listResp: &notificationrepo.InboxPage{
			Total: 1,
			Items: []notificationrepo.InboxAggregate{
				{
					Inbox: model.NotificationInbox{Base: model.Base{ID: 7}, EventID: 10},
					Event: model.NotificationEvent{
						Base: model.Base{ID: 10},
						Type: notificationservice.EventTypeSystemNotice,
					},
					ActorUser: nil,
				},
			},
		},
	}
	service := notificationservice.NewInboxService(repo, nil, nil, nil)

	resp, err := service.List(3, dto.NotificationListReq{Page: 1, PageSize: 10})

	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	assert.Nil(t, resp.List[0].ActorUserID)
	assert.Nil(t, resp.List[0].ActorUser)
}

// 标记已读时仓储影响 0 行表示通知不属于该用户，应返回 ErrNotificationNotFound。
func TestInboxService_MarkRead_RejectsNotOwned(t *testing.T) {
	repo := &fakeInboxRepo{markReadAffected: 0}
	service := notificationservice.NewInboxService(repo, nil, nil, nil)

	err := service.MarkRead(3, 7)

	assert.ErrorIs(t, err, notificationservice.ErrNotificationNotFound)
}

// ids 为空表示全部未读，服务应把空 ids 透传给仓储并返回受影响数量。
func TestInboxService_MarkAllRead_SupportsAllWhenIDsEmpty(t *testing.T) {
	repo := &fakeInboxRepo{markAllReadAffected: 4}
	service := notificationservice.NewInboxService(repo, nil, nil, nil)

	resp, err := service.MarkAllRead(3, nil)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, repo.markAllReadIDs)
	assert.Equal(t, int64(4), resp.Updated)
}

type fakeEngagementResolver struct {
	engagements     map[notificationrepo.SourceEngagementKey]notificationrepo.SourceEngagement
	replyCommentIDs map[uint]uint
}

func (f *fakeEngagementResolver) BatchSourceEngagement(_ context.Context, _ uint, refs []notificationrepo.SourceEngagementRef) (map[notificationrepo.SourceEngagementKey]notificationrepo.SourceEngagement, error) {
	out := make(map[notificationrepo.SourceEngagementKey]notificationrepo.SourceEngagement, len(refs))
	for _, ref := range refs {
		key := notificationrepo.SourceEngagementKey{SourceType: ref.SourceType, SourceID: ref.SourceID}
		if eng, ok := f.engagements[key]; ok {
			out[key] = eng
		}
	}
	return out, nil
}

func (f *fakeEngagementResolver) BatchReplyCommentIDs(_ context.Context, refs []notificationrepo.SourceEngagementRef) (map[uint]uint, error) {
	out := make(map[uint]uint, len(refs))
	for _, ref := range refs {
		if commentID, ok := f.replyCommentIDs[ref.SourceID]; ok {
			out[ref.SourceID] = commentID
		}
	}
	return out, nil
}

type fakeDeletedResolver struct {
	deleted map[notificationrepo.ObjectDeletedKey]bool
	refs    []notificationrepo.ObjectDeletedRef
}

func (f *fakeDeletedResolver) BatchObjectDeleted(_ context.Context, refs []notificationrepo.ObjectDeletedRef) (map[notificationrepo.ObjectDeletedKey]bool, error) {
	f.refs = append(f.refs, refs...)
	out := make(map[notificationrepo.ObjectDeletedKey]bool, len(refs))
	for _, ref := range refs {
		key := notificationrepo.ObjectDeletedKey{ObjectType: ref.ObjectType, ObjectID: ref.ObjectID, RootType: ref.RootType}
		out[key] = f.deleted[key]
	}
	return out, nil
}

func TestInboxService_List_FillsDeletedStatus(t *testing.T) {
	repo := &fakeInboxRepo{
		listResp: &notificationrepo.InboxPage{
			Total: 2,
			Items: []notificationrepo.InboxAggregate{
				{
					Inbox: model.NotificationInbox{Base: model.Base{ID: 1}, EventID: 10},
					Event: model.NotificationEvent{
						SourceType: "comment",
						SourceID:   99,
						RootType:   "article",
						RootID:     7,
					},
				},
				{
					Inbox: model.NotificationInbox{Base: model.Base{ID: 2}, EventID: 11},
					Event: model.NotificationEvent{
						SourceType: "reply",
						SourceID:   100,
						RootType:   "guestbook",
						RootID:     12,
					},
				},
			},
		},
	}
	status := &fakeDeletedResolver{
		deleted: map[notificationrepo.ObjectDeletedKey]bool{
			{ObjectType: "comment", ObjectID: 99, RootType: "article"}:  true,
			{ObjectType: "article", ObjectID: 7}:                        false,
			{ObjectType: "reply", ObjectID: 100, RootType: "guestbook"}: false,
			{ObjectType: "guestbook", ObjectID: 12}:                     true,
		},
	}
	service := notificationservice.NewInboxService(repo, status, nil, nil)

	resp, err := service.List(3, dto.NotificationListReq{Page: 1, PageSize: 10})

	require.NoError(t, err)
	require.Len(t, resp.List, 2)
	assert.True(t, resp.List[0].SourceDeleted)
	assert.False(t, resp.List[0].RootDeleted)
	assert.False(t, resp.List[1].SourceDeleted)
	assert.True(t, resp.List[1].RootDeleted)
	assert.Contains(t, status.refs, notificationrepo.ObjectDeletedRef{ObjectType: "comment", ObjectID: 99, RootType: "article"})
	assert.Contains(t, status.refs, notificationrepo.ObjectDeletedRef{ObjectType: "article", ObjectID: 7})
	assert.Contains(t, status.refs, notificationrepo.ObjectDeletedRef{ObjectType: "reply", ObjectID: 100, RootType: "guestbook"})
	assert.Contains(t, status.refs, notificationrepo.ObjectDeletedRef{ObjectType: "guestbook", ObjectID: 12})
}

func TestInboxService_List_FillsEngagementAndReplyCommentID(t *testing.T) {
	repo := &fakeInboxRepo{
		listResp: &notificationrepo.InboxPage{
			Total: 2,
			Items: []notificationrepo.InboxAggregate{
				{
					Inbox: model.NotificationInbox{Base: model.Base{ID: 1}, EventID: 10},
					Event: model.NotificationEvent{
						SourceType: "comment",
						SourceID:   99,
						RootType:   "article",
						RootID:     7,
					},
				},
				{
					Inbox: model.NotificationInbox{Base: model.Base{ID: 2}, EventID: 11},
					Event: model.NotificationEvent{
						SourceType: "reply",
						SourceID:   100,
						RootType:   "article",
						RootID:     7,
					},
				},
			},
		},
	}
	engagement := &fakeEngagementResolver{
		engagements: map[notificationrepo.SourceEngagementKey]notificationrepo.SourceEngagement{
			{SourceType: "comment", SourceID: 99}: {LikeCount: 3, IsLiked: true, ReplyCount: 5},
			{SourceType: "reply", SourceID: 100}:  {LikeCount: 1, IsLiked: false, ReplyCount: 2},
		},
		replyCommentIDs: map[uint]uint{100: 99},
	}
	service := notificationservice.NewInboxService(repo, nil, engagement, nil)

	resp, err := service.List(3, dto.NotificationListReq{Page: 1, PageSize: 10})

	require.NoError(t, err)
	require.Len(t, resp.List, 2)
	require.NotNil(t, resp.List[0].LikeCount)
	assert.Equal(t, int64(3), *resp.List[0].LikeCount)
	require.NotNil(t, resp.List[0].IsLiked)
	assert.True(t, *resp.List[0].IsLiked)
	require.NotNil(t, resp.List[0].ReplyCount)
	assert.Equal(t, int64(5), *resp.List[0].ReplyCount)

	require.NotNil(t, resp.List[1].LikeCount)
	assert.Equal(t, int64(1), *resp.List[1].LikeCount)
	require.NotNil(t, resp.List[1].ReplyCount)
	assert.Equal(t, int64(2), *resp.List[1].ReplyCount)
	require.NotNil(t, resp.List[1].Metadata)
	assert.Contains(t, *resp.List[1].Metadata, `"comment_id":99`)
}
