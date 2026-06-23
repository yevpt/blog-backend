package notification

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/vpt/blog-backend/internal/model"
	notificationrepo "github.com/vpt/blog-backend/internal/repository/notification"
)

// OwnerLookup 解析某个对象的归属用户，例如 (moment,12) -> 碎语作者。
// 由业务侧适配既有领域仓储实现，避免通知层直接查询业务表。
type OwnerLookup interface {
	OwnerOf(ctx context.Context, objectType string, objectID uint) (uint, bool, error)
}

// DefaultPreferenceResolver 基于 notification_preference 解析偏好，无配置时默认站内与邮件都开。
type DefaultPreferenceResolver struct {
	repo notificationrepo.PreferenceRepository
}

// NewPreferenceResolver 创建默认偏好解析器。
func NewPreferenceResolver(repo notificationrepo.PreferenceRepository) *DefaultPreferenceResolver {
	return &DefaultPreferenceResolver{repo: repo}
}

// Resolve 读取用户偏好；无记录时回退为站内与邮件均开启，由总开关与额度进一步约束。
func (r *DefaultPreferenceResolver) Resolve(ctx context.Context, userID uint, eventType string) (Preference, error) {
	pref, err := r.repo.GetPreference(ctx, userID, eventType)
	if err != nil {
		return Preference{}, err
	}
	if pref == nil {
		return Preference{InAppEnabled: true, EmailEnabled: true}, nil
	}
	return Preference{InAppEnabled: pref.InAppEnabled, EmailEnabled: pref.EmailEnabled}, nil
}

// DefaultRecipientResolver 按事件类型解析接收人。
//
// 优先使用 metadata 中显式指定的接收人（回复、系统通知由发布方写入）；
// 否则把评论、点赞、留言等事件投递给根对象作者。
type DefaultRecipientResolver struct {
	owners OwnerLookup
}

// NewRecipientResolver 创建默认接收人解析器。
func NewRecipientResolver(owners OwnerLookup) *DefaultRecipientResolver {
	return &DefaultRecipientResolver{owners: owners}
}

// Resolve 解析事件接收人。
func (r *DefaultRecipientResolver) Resolve(ctx context.Context, event model.NotificationEvent) ([]uint, error) {
	// 显式接收人优先（回复指向被回复人、系统通知指向目标用户）。
	if ids := explicitRecipients(event.MetadataJSON); len(ids) > 0 {
		return ids, nil
	}

	// 系统通知没有根对象作者可推导，只接受显式接收人。
	if event.Type == EventTypeSystemNotice {
		return nil, nil
	}

	// 其余事件投递给根对象作者。
	owner, ok, err := r.owners.OwnerOf(ctx, event.RootType, event.RootID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return []uint{owner}, nil
}

// recipientMetadata 是 metadata_json 中与接收人相关的可选字段。
type recipientMetadata struct {
	RecipientUserIDs []uint `json:"recipient_user_ids"`
}

// BuildRecipientMetadata 把显式接收人列表编码为事件 metadata JSON；空列表返回 nil。
// 供回复、留言等已知接收人的业务在发布事件时写入，分发时优先于按归属解析。
func BuildRecipientMetadata(userIDs ...uint) *string {
	if len(userIDs) == 0 {
		return nil
	}
	encoded, err := json.Marshal(recipientMetadata{RecipientUserIDs: userIDs})
	if err != nil {
		return nil
	}
	value := string(encoded)
	return &value
}

// eventMetadata 是事件 metadata_json 的可选扩展字段。
type eventMetadata struct {
	RecipientUserIDs []uint                `json:"recipient_user_ids,omitempty"`
	CommentID        uint                  `json:"comment_id,omitempty"`
	SourceSnapshot   *NotificationSnapshot `json:"source_snapshot,omitempty"`
	RootSnapshot     *NotificationSnapshot `json:"root_snapshot,omitempty"`
	QuoteSnapshot    *NotificationSnapshot `json:"quote_snapshot,omitempty"`
}

// NotificationSnapshot 记录通知发生时对象的展示快照，供对象删除后继续展示上下文。
type NotificationSnapshot struct {
	Type    string `json:"type,omitempty"`
	ID      uint   `json:"id,omitempty"`
	Title   string `json:"title,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
}

// BuildCommentLikedMetadata 编码评论点赞通知的接收人与被点赞评论摘要。
func BuildCommentLikedMetadata(recipientUserID uint, quotedExcerpt string) *string {
	if recipientUserID == 0 && strings.TrimSpace(quotedExcerpt) == "" {
		return nil
	}
	meta := eventMetadata{}
	if recipientUserID != 0 {
		meta.RecipientUserIDs = []uint{recipientUserID}
	}
	meta.SourceSnapshot = snapshot("comment", 0, "", quotedExcerpt)
	return marshalEventMetadata(meta)
}

// BuildReplyCreatedMetadata 编码回复通知的接收人、父评论 ID、根对象快照与被引用内容摘要。
func BuildReplyCreatedMetadata(recipientUserID, commentID uint, replyExcerpt string, root *NotificationSnapshot, quotedExcerpt string) *string {
	if recipientUserID == 0 && commentID == 0 && strings.TrimSpace(replyExcerpt) == "" && strings.TrimSpace(quotedExcerpt) == "" && (root == nil || snapshotEmpty(*root)) {
		return nil
	}
	meta := eventMetadata{}
	if recipientUserID != 0 {
		meta.RecipientUserIDs = []uint{recipientUserID}
	}
	if commentID != 0 {
		meta.CommentID = commentID
	}
	meta.SourceSnapshot = snapshot("reply", 0, "", replyExcerpt)
	if root != nil {
		rootSnapshot := normalizeSnapshot(*root)
		if !snapshotEmpty(rootSnapshot) {
			meta.RootSnapshot = &rootSnapshot
		}
	}
	meta.QuoteSnapshot = snapshot("comment", commentID, "", quotedExcerpt)
	return marshalEventMetadata(meta)
}

// BuildReplyLikedMetadata 编码回复点赞通知的接收人、父评论 ID 与根对象快照。
func BuildReplyLikedMetadata(recipientUserID, commentID uint, replyExcerpt string, root *NotificationSnapshot) *string {
	if recipientUserID == 0 && commentID == 0 && strings.TrimSpace(replyExcerpt) == "" && (root == nil || snapshotEmpty(*root)) {
		return nil
	}
	meta := eventMetadata{}
	if recipientUserID != 0 {
		meta.RecipientUserIDs = []uint{recipientUserID}
	}
	if commentID != 0 {
		meta.CommentID = commentID
	}
	meta.SourceSnapshot = snapshot("reply", 0, "", replyExcerpt)
	if root != nil {
		rootSnapshot := normalizeSnapshot(*root)
		if !snapshotEmpty(rootSnapshot) {
			meta.RootSnapshot = &rootSnapshot
		}
	}
	return marshalEventMetadata(meta)
}

// BuildSourceRootMetadata 编码来源对象与根对象快照。
func BuildSourceRootMetadata(source NotificationSnapshot, root *NotificationSnapshot, recipientUserIDs ...uint) *string {
	meta := eventMetadata{}
	if len(recipientUserIDs) > 0 {
		meta.RecipientUserIDs = recipientUserIDs
	}
	source = normalizeSnapshot(source)
	if !snapshotEmpty(source) {
		meta.SourceSnapshot = &source
	}
	if root != nil {
		rootSnapshot := normalizeSnapshot(*root)
		if !snapshotEmpty(rootSnapshot) {
			meta.RootSnapshot = &rootSnapshot
		}
	}
	if len(meta.RecipientUserIDs) == 0 && meta.SourceSnapshot == nil && meta.RootSnapshot == nil {
		return nil
	}
	return marshalEventMetadata(meta)
}

func snapshot(objectType string, id uint, title string, excerpt string) *NotificationSnapshot {
	s := normalizeSnapshot(NotificationSnapshot{
		Type:    objectType,
		ID:      id,
		Title:   title,
		Excerpt: excerpt,
	})
	if snapshotEmpty(s) {
		return nil
	}
	return &s
}

func normalizeSnapshot(s NotificationSnapshot) NotificationSnapshot {
	return NotificationSnapshot{
		Type:    strings.TrimSpace(s.Type),
		ID:      s.ID,
		Title:   truncateRunes(strings.TrimSpace(s.Title), maxTitleRunes),
		Excerpt: truncateRunes(strings.TrimSpace(s.Excerpt), maxExcerptRunes),
	}
}

func snapshotEmpty(s NotificationSnapshot) bool {
	return s.Type == "" && s.ID == 0 && strings.TrimSpace(s.Title) == "" && strings.TrimSpace(s.Excerpt) == ""
}

func marshalEventMetadata(meta eventMetadata) *string {
	encoded, err := json.Marshal(meta)
	if err != nil {
		return nil
	}
	value := string(encoded)
	return &value
}

// explicitRecipients 从事件 metadata 解析显式接收人列表，解析失败或缺失时返回空。
func explicitRecipients(metadataJSON *string) []uint {
	if metadataJSON == nil || *metadataJSON == "" {
		return nil
	}
	var meta recipientMetadata
	if err := json.Unmarshal([]byte(*metadataJSON), &meta); err != nil {
		return nil
	}
	return meta.RecipientUserIDs
}
