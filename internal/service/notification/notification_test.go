package notification_test

import (
	"context"
	"errors"
	"fmt"
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

// fakeRootResolver 按 (rootType, rootID) 预置展示标题与正文摘录，记录调用次数以验证去重。
type fakeRootResolver struct {
	labels         map[string]string
	excerpts       map[string]string
	calls          int
	excerptCalls   int
	callErr        error
	excerptCallErr error
}

func (f *fakeRootResolver) RootSnapshotOf(_ context.Context, rootType string, rootID uint) (string, error) {
	f.calls++
	if f.callErr != nil {
		return "", f.callErr
	}
	return f.labels[fmt.Sprintf("%s:%d", rootType, rootID)], nil
}

func (f *fakeRootResolver) RootExcerptOf(_ context.Context, rootType string, rootID uint) (string, error) {
	f.excerptCalls++
	if f.excerptCallErr != nil {
		return "", f.excerptCallErr
	}
	return f.excerpts[fmt.Sprintf("%s:%d", rootType, rootID)], nil
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

// 列表应在读取时按根对象解析展示标题并去重，命中 article/moment 时填充 RootTitle。
func TestInboxService_List_ResolvesRootTitleWithDedup(t *testing.T) {
	repo := &fakeInboxRepo{
		listResp: &notificationrepo.InboxPage{
			Total: 3,
			Items: []notificationrepo.InboxAggregate{
				{
					Inbox: model.NotificationInbox{Base: model.Base{ID: 1}, EventID: 10},
					Event: model.NotificationEvent{
						Base:       model.Base{ID: 10},
						Type:       notificationservice.EventTypeCommentCreated,
						SourceType: "comment",
						SourceID:   99,
						RootType:   "article",
						RootID:     7,
					},
				},
				{
					// 同一篇文章的第二条评论，应复用去重结果。
					Inbox: model.NotificationInbox{Base: model.Base{ID: 2}, EventID: 11},
					Event: model.NotificationEvent{
						Base:       model.Base{ID: 11},
						Type:       notificationservice.EventTypeReplyCreated,
						SourceType: "reply",
						SourceID:   100,
						RootType:   "article",
						RootID:     7,
					},
				},
				{
					// 留言板根对象无展示标题，RootTitle 应为空。
					Inbox: model.NotificationInbox{Base: model.Base{ID: 3}, EventID: 12},
					Event: model.NotificationEvent{
						Base:       model.Base{ID: 12},
						Type:       notificationservice.EventTypeGuestbookCreated,
						SourceType: "guestbook",
						SourceID:   5,
						RootType:   "guestbook",
						RootID:     1,
					},
				},
			},
		},
	}
	roots := &fakeRootResolver{labels: map[string]string{"article:7": "我的第一篇文章"}}
	service := notificationservice.NewInboxService(repo, roots, nil, nil)

	resp, err := service.List(3, dto.NotificationListReq{Page: 1, PageSize: 10})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.List, 3)
	// 前两条命中同一文章，RootTitle 一致且只解析一次。
	require.NotNil(t, resp.List[0].RootTitle)
	assert.Equal(t, "我的第一篇文章", *resp.List[0].RootTitle)
	require.NotNil(t, resp.List[1].RootTitle)
	assert.Equal(t, "我的第一篇文章", *resp.List[1].RootTitle)
	// 留言板无展示标题。
	assert.Nil(t, resp.List[2].RootTitle)
	// 去重：article:7 仅解析一次，guestbook 不走解析。
	assert.Equal(t, 1, roots.calls)
}

// roots 为 nil 时退化为不填充 RootTitle，保持旧行为。
func TestInboxService_List_SkipsRootTitleWhenResolverNil(t *testing.T) {
	repo := &fakeInboxRepo{
		listResp: &notificationrepo.InboxPage{
			Total: 1,
			Items: []notificationrepo.InboxAggregate{
				{
					Inbox: model.NotificationInbox{Base: model.Base{ID: 1}, EventID: 10},
					Event: model.NotificationEvent{RootType: "article", RootID: 7},
				},
			},
		},
	}
	service := notificationservice.NewInboxService(repo, nil, nil, nil)

	resp, err := service.List(3, dto.NotificationListReq{Page: 1, PageSize: 10})

	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	assert.Nil(t, resp.List[0].RootTitle)
}

// 根对象解析出错时应把错误透传出 List。
func TestInboxService_List_PropagatesRootResolveError(t *testing.T) {
	repo := &fakeInboxRepo{
		listResp: &notificationrepo.InboxPage{
			Total: 1,
			Items: []notificationrepo.InboxAggregate{
				{
					Inbox: model.NotificationInbox{Base: model.Base{ID: 1}, EventID: 10},
					Event: model.NotificationEvent{RootType: "article", RootID: 7},
				},
			},
		},
	}
	roots := &fakeRootResolver{callErr: errors.New("boom")}
	service := notificationservice.NewInboxService(repo, roots, nil, nil)

	_, err := service.List(3, dto.NotificationListReq{Page: 1, PageSize: 10})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

// 文章点赞通知（ContentExcerpt 为空）应填充 root_excerpt，且按 rootID 去重。
// 同一篇文章的评论通知（ContentExcerpt 非空）不应填充 root_excerpt，避免重复。
func TestInboxService_List_FillsRootExcerptForArticleWithoutContentExcerpt(t *testing.T) {
	repo := &fakeInboxRepo{
		listResp: &notificationrepo.InboxPage{
			Total: 3,
			Items: []notificationrepo.InboxAggregate{
				{
					// 文章点赞：无 ContentExcerpt，应填充 root_excerpt。
					Inbox: model.NotificationInbox{Base: model.Base{ID: 1}, EventID: 10},
					Event: model.NotificationEvent{
						Base:       model.Base{ID: 10},
						Type:       notificationservice.EventTypeArticleLiked,
						SourceType: "article",
						SourceID:   7,
						RootType:   "article",
						RootID:     7,
					},
				},
				{
					// 同一篇文章的评论通知：ContentExcerpt 非空，不应填充 root_excerpt。
					Inbox: model.NotificationInbox{Base: model.Base{ID: 2}, EventID: 11},
					Event: model.NotificationEvent{
						Base:           model.Base{ID: 11},
						Type:           notificationservice.EventTypeCommentCreated,
						SourceType:     "comment",
						SourceID:       99,
						RootType:       "article",
						RootID:         7,
						ContentExcerpt: "好文章",
					},
				},
				{
					// 碎语点赞：RootType 非 article，不应填充 root_excerpt。
					Inbox: model.NotificationInbox{Base: model.Base{ID: 3}, EventID: 12},
					Event: model.NotificationEvent{
						Base:       model.Base{ID: 12},
						Type:       notificationservice.EventTypeMomentLiked,
						SourceType: "moment",
						SourceID:   5,
						RootType:   "moment",
						RootID:     5,
					},
				},
			},
		},
	}
	roots := &fakeRootResolver{
		labels:   map[string]string{"article:7": "我的第一篇文章"},
		excerpts: map[string]string{"article:7": "这是文章正文的前面一部分内容"},
	}
	service := notificationservice.NewInboxService(repo, roots, nil, nil)

	resp, err := service.List(3, dto.NotificationListReq{Page: 1, PageSize: 10})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.List, 3)
	// 文章点赞：填充 root_excerpt。
	require.NotNil(t, resp.List[0].RootExcerpt)
	assert.Equal(t, "这是文章正文的前面一部分内容", *resp.List[0].RootExcerpt)
	// 文章评论：不填充 root_excerpt（已有 content_excerpt）。
	assert.Nil(t, resp.List[1].RootExcerpt)
	// 碎语点赞：不填充 root_excerpt。
	assert.Nil(t, resp.List[2].RootExcerpt)
	// 去重：article:7 的正文摘录只解析一次（点赞条目解析，评论条目跳过）。
	assert.Equal(t, 1, roots.excerptCalls)
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
