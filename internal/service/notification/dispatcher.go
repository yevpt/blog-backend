package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	notificationrepo "github.com/vpt/blog-backend/internal/repository/notification"
)

// 邮件用途与默认优先级。
const (
	PurposeNotification = "notification" // 互动通知摘要邮件
	// PriorityNotification 通知邮件默认低优先级，数字越大越靠后，不挤占验证码等高优先邮件。
	PriorityNotification = 100
)

// emailEligibleEventTypes 是首版允许进入邮件队列的事件类型。
// 评论、回复、留言进邮件；点赞等仅站内通知，不发邮件以控制邮件量。
var emailEligibleEventTypes = map[string]struct{}{
	EventTypeCommentCreated:   {},
	EventTypeReplyCreated:     {},
	EventTypeGuestbookCreated: {},
}

// 分发默认参数。
const (
	defaultLeaseSeconds = 300              // 事件租约默认 5 分钟
	defaultRetryBackoff = 60 * time.Second // 失败后默认 1 分钟后重试
)

// RecipientResolver 把一个事件解析为接收人用户 ID 集合。
type RecipientResolver interface {
	// Resolve 返回事件应投递到的接收人用户 ID，去重由调用方负责。
	Resolve(ctx context.Context, event model.NotificationEvent) ([]uint, error)
}

// PreferenceResolver 解析用户对某事件类型的投递偏好（含默认回退）。
type PreferenceResolver interface {
	Resolve(ctx context.Context, userID uint, eventType string) (Preference, error)
}

// Preference 用户对某事件类型的投递偏好。
type Preference struct {
	InAppEnabled bool // 是否接收站内通知
	EmailEnabled bool // 是否接收邮件通知
}

// UserDirectory 提供接收人的邮件投递资料。
type UserDirectory interface {
	// MailProfile 返回用户邮箱与总邮件开关（user_setting.receive_mail）。
	MailProfile(ctx context.Context, userID uint) (email string, canReceiveMail bool, err error)
}

// Dispatcher 通知事件分发器：领取待分发事件，解析接收人，写收件箱并按偏好入队邮件任务。
// 每个事件处理保持幂等，可被 worker 反复安全调用。
type Dispatcher struct {
	events       notificationrepo.EventRepository
	inbox        notificationrepo.InboxRepository
	tasks        notificationrepo.EmailTaskRepository
	recipients   RecipientResolver
	preferences  PreferenceResolver
	directory    UserDirectory
	leaseSeconds int
	retryBackoff time.Duration
}

// NewDispatcher 创建事件分发器。
func NewDispatcher(
	repo notificationrepo.Repository,
	recipients RecipientResolver,
	preferences PreferenceResolver,
	directory UserDirectory,
) *Dispatcher {
	return &Dispatcher{
		events:       repo,
		inbox:        repo,
		tasks:        repo,
		recipients:   recipients,
		preferences:  preferences,
		directory:    directory,
		leaseSeconds: defaultLeaseSeconds,
		retryBackoff: defaultRetryBackoff,
	}
}

// DispatchOnce 领取一批待分发事件并逐个处理，返回成功处理的事件数。
// 单个事件失败只回退该事件并继续，不影响其余事件。
func (d *Dispatcher) DispatchOnce(ctx context.Context, workerID string, limit int) (int, error) {
	events, err := d.events.LeasePendingEvents(ctx, workerID, d.leaseSeconds, limit)
	if err != nil {
		return 0, err
	}

	processed := 0
	for _, event := range events {
		if err := d.dispatchEvent(ctx, event); err != nil {
			// 失败回退为 pending，并退避一段时间后重试，错误信息落库便于排查。
			next := time.Now().Add(d.retryBackoff)
			_ = d.events.MarkEventRetry(ctx, event.ID, next, err.Error())
			continue
		}
		if err := d.events.MarkEventDone(ctx, event.ID); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

// dispatchEvent 处理单个事件：解析接收人，按偏好写收件箱与邮件任务。
func (d *Dispatcher) dispatchEvent(ctx context.Context, event model.NotificationEvent) error {
	recipients, err := d.recipients.Resolve(ctx, event)
	if err != nil {
		return err
	}

	for _, userID := range recipients {
		// 操作人就是接收人时不通知自己。
		if event.ActorUserID != nil && *event.ActorUserID == userID {
			continue
		}
		if err := d.deliverTo(ctx, event, userID); err != nil {
			return err
		}
	}
	return nil
}

// deliverTo 按用户偏好投递站内通知与邮件任务。
func (d *Dispatcher) deliverTo(ctx context.Context, event model.NotificationEvent, userID uint) error {
	pref, err := d.preferences.Resolve(ctx, userID, event.Type)
	if err != nil {
		return err
	}

	// 站内通知：偏好开启则幂等写收件箱。
	if pref.InAppEnabled {
		inbox := &model.NotificationInbox{
			EventID:         event.ID,
			RecipientUserID: userID,
			DeliveredAt:     time.Now(),
		}
		if _, err := d.inbox.CreateInbox(ctx, inbox); err != nil {
			return err
		}
	}

	// 邮件通知：事件类型在白名单 + 偏好开启 + 总开关允许 + 有邮箱，才幂等入队邮件任务。
	if _, eligible := emailEligibleEventTypes[event.Type]; !eligible {
		return nil
	}
	if !pref.EmailEnabled {
		return nil
	}
	email, canReceiveMail, err := d.directory.MailProfile(ctx, userID)
	if err != nil {
		return err
	}
	if !canReceiveMail || email == "" {
		return nil
	}

	now := time.Now()
	task := &model.NotificationEmailTask{
		EventID:         event.ID,
		RecipientUserID: userID,
		ActorUserID:     event.ActorUserID,
		ToEmail:         email,
		EventType:       event.Type,
		Purpose:         PurposeNotification,
		Priority:        PriorityNotification,
		Status:          notificationrepo.EmailTaskStatusPending,
		AvailableAt:     now,
		NextAttemptAt:   now,
		IdempotencyKey:  emailTaskIdempotencyKey(event.ID, userID),
	}
	_, err = d.tasks.CreateEmailTask(ctx, task)
	return err
}

// emailTaskIdempotencyKey 由事件与接收人拼出稳定幂等键，重复分发不会重复入队。
func emailTaskIdempotencyKey(eventID, userID uint) string {
	return fmt.Sprintf("event:%d:user:%d", eventID, userID)
}
