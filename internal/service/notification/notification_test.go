package notification_test

import (
	"context"
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
				},
			},
		},
	}
	service := notificationservice.NewInboxService(repo)

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
	assert.Equal(t, int64(1), resp.Total)
}

// 标记已读时仓储影响 0 行表示通知不属于该用户，应返回 ErrNotificationNotFound。
func TestInboxService_MarkRead_RejectsNotOwned(t *testing.T) {
	repo := &fakeInboxRepo{markReadAffected: 0}
	service := notificationservice.NewInboxService(repo)

	err := service.MarkRead(3, 7)

	assert.ErrorIs(t, err, notificationservice.ErrNotificationNotFound)
}

// ids 为空表示全部未读，服务应把空 ids 透传给仓储并返回受影响数量。
func TestInboxService_MarkAllRead_SupportsAllWhenIDsEmpty(t *testing.T) {
	repo := &fakeInboxRepo{markAllReadAffected: 4}
	service := notificationservice.NewInboxService(repo)

	resp, err := service.MarkAllRead(3, nil)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, repo.markAllReadIDs)
	assert.Equal(t, int64(4), resp.Updated)
}
