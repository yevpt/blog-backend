package notification

import (
	"context"
	"encoding/json"

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
