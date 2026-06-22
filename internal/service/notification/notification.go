// Package notification 提供通知系统的业务服务层。
//
// 业务方通过 Publisher 仅产生 notification_event，事件如何投递收件箱、
// 是否进入邮件队列由 dispatcher 决定；收件箱的读取与已读则由 InboxService 承担。
// 本层只依赖 repository 接口，不直接接触 GORM。
package notification

import (
	"context"
	"errors"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	notificationrepo "github.com/vpt/blog-backend/internal/repository/notification"
)

// 事件类型，发布时据此校验合法性。
const (
	EventTypeCommentCreated   = "comment_created"   // 评论
	EventTypeReplyCreated     = "reply_created"     // 回复
	EventTypeArticleLiked     = "article_liked"     // 文章点赞
	EventTypeMomentLiked      = "moment_liked"      // 碎语点赞
	EventTypeGuestbookCreated = "guestbook_created" // 留言
	EventTypeGuestbookLiked   = "guestbook_liked"   // 留言点赞
	EventTypeSystemNotice     = "system_notice"     // 系统通知
	EventTypeLegacyNotice     = "legacy_notice"     // 旧数据迁移保留类型
)

// 字段快照上限，与 model 列宽保持一致，避免写库截断报错。
const (
	maxTitleRunes   = 120
	maxExcerptRunes = 500
)

var (
	// ErrInvalidEventType 表示发布的事件类型不在允许集合内。
	ErrInvalidEventType = errors.New("非法的通知事件类型")
	// ErrNotificationNotFound 表示通知不存在或不属于当前用户。
	ErrNotificationNotFound = errors.New("通知不存在")
)

// allowedEventTypes 是允许发布的事件类型集合。
var allowedEventTypes = map[string]struct{}{
	EventTypeCommentCreated:   {},
	EventTypeReplyCreated:     {},
	EventTypeArticleLiked:     {},
	EventTypeMomentLiked:      {},
	EventTypeGuestbookCreated: {},
	EventTypeGuestbookLiked:   {},
	EventTypeSystemNotice:     {},
	EventTypeLegacyNotice:     {},
}

// PublishEvent 业务方发布通知事件的入参，字段对应事件事实与展示快照。
type PublishEvent struct {
	Type           string  // 事件类型，必须在允许集合内
	ActorUserID    *uint   // 操作人，系统消息可为空
	SourceType     string  // 直接对象类型，如 comment
	SourceID       uint    // 直接对象 ID
	RootType       string  // 根对象类型，如 moment
	RootID         uint    // 根对象 ID
	Title          string  // 标题快照
	ContentExcerpt string  // 内容摘要快照
	Metadata       *string // 跳转与扩展信息的 JSON
}

// Publisher 面向业务的通知事件发布器。
type Publisher interface {
	// Publish 校验并落库一条待分发事件，仅创建 notification_event，不直接写收件箱。
	Publish(ctx context.Context, event PublishEvent) (*model.NotificationEvent, error)
}

// InboxService 站内收件箱用例：列表、未读数、已读、删除。
type InboxService interface {
	List(userID uint, req dto.NotificationListReq) (*dto.NotificationPageResp, error)
	UnreadCount(userID uint) (*dto.NotificationUnreadCountResp, error)
	MarkRead(userID uint, id uint) error
	MarkAllRead(userID uint, ids []uint) (*dto.NotificationReadResp, error)
	Delete(userID uint, id uint) error
}

type publisherService struct {
	repo notificationrepo.EventRepository
}

// NewPublisher 创建通知事件发布器。
func NewPublisher(repo notificationrepo.EventRepository) Publisher {
	return &publisherService{repo: repo}
}

type inboxService struct {
	repo notificationrepo.InboxRepository
}

// NewInboxService 创建收件箱业务服务。
func NewInboxService(repo notificationrepo.InboxRepository) InboxService {
	return &inboxService{repo: repo}
}
