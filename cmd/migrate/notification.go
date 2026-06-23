package main

import (
	"encoding/json"
	"fmt"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// legacyEventType 把旧 message.type 规范化为 v2 事件类型，未知类型回退 legacy_notice。
func legacyEventType(oldType string) string {
	switch oldType {
	case "post_like", "article_like":
		return "article_liked"
	case "say_like", "moment_like":
		return "moment_liked"
	case "comment":
		return "comment_created"
	case "moment_comment", "say":
		return "comment_created"
	case "comment_reply":
		return "reply_created"
	case "guestBook":
		return "guestbook_created"
	case "guestBook_reply":
		return "reply_created"
	default:
		return "legacy_notice"
	}
}

// seedNotificationPolicies 写入默认邮件额度策略与角色额度策略，命中唯一键时跳过（幂等）。
func seedNotificationPolicies(dst *gorm.DB) error {
	policies := []model.EmailQuotaPolicy{
		{Purpose: "register_code", DailyLimit: 200, ReservedMin: 50, Priority: 1, MaxPerMinute: 5, MaxPerHour: 100, Enabled: true},
		{Purpose: "password_reset", DailyLimit: 200, ReservedMin: 30, Priority: 1, MaxPerMinute: 5, MaxPerHour: 100, Enabled: true},
		{Purpose: "security", DailyLimit: 100, ReservedMin: 10, Priority: 5, MaxPerMinute: 5, MaxPerHour: 60, Enabled: true},
		{Purpose: "notification", DailyLimit: 150, ReservedMin: 0, Priority: 100, MaxPerMinute: 5, MaxPerHour: 80, Enabled: true},
		{Purpose: "admin_notice", DailyLimit: 100, ReservedMin: 0, Priority: 50, MaxPerMinute: 5, MaxPerHour: 60, Enabled: true},
	}
	if err := dst.Clauses(clause.OnConflict{DoNothing: true}).Create(&policies).Error; err != nil {
		return fmt.Errorf("seed email_quota_policy: %w", err)
	}

	// actor/recipient 双维度的角色额度，管理员额度更高但仍受限。
	roleQuotas := []model.EmailRoleQuotaPolicy{
		{Role: "normal", ScopeType: "actor", DailyLimit: 30, MaxPerHour: 0, Enabled: true},
		{Role: "vip", ScopeType: "actor", DailyLimit: 100, MaxPerHour: 0, Enabled: true},
		{Role: "admin", ScopeType: "actor", DailyLimit: 300, MaxPerHour: 0, Enabled: true},
		{Role: "normal", ScopeType: "recipient", DailyLimit: 5, MaxPerHour: 0, Enabled: true},
		{Role: "vip", ScopeType: "recipient", DailyLimit: 20, MaxPerHour: 0, Enabled: true},
		{Role: "admin", ScopeType: "recipient", DailyLimit: 50, MaxPerHour: 0, Enabled: true},
	}
	if err := dst.Clauses(clause.OnConflict{DoNothing: true}).Create(&roleQuotas).Error; err != nil {
		return fmt.Errorf("seed email_role_quota_policy: %w", err)
	}
	return nil
}

// migrateNotifications 把已迁入 Go 库的 message/user_message 转换为 v2 通知事件与收件箱。
//
// 旧 message 映射为 notification_event（type 规范化，原始信息存入 metadata_json）；
// 旧 user_message 通过 message_id 关联到事件后映射为 notification_inbox，孤儿记录跳过。
// 收件箱唯一约束保证重复执行不产生重复投递。
func migrateNotifications(dst *gorm.DB) error {
	// 回填可重复执行：先清空 v2 表，保证多次运行（含上次部分失败）后结果一致。
	if err := dst.Exec("DELETE FROM `notification_inbox`").Error; err != nil {
		return fmt.Errorf("清空 notification_inbox: %w", err)
	}
	if err := dst.Exec("DELETE FROM `notification_event`").Error; err != nil {
		return fmt.Errorf("清空 notification_event: %w", err)
	}

	var messages []model.Message
	if err := dst.Order("id").Find(&messages).Error; err != nil {
		return fmt.Errorf("读取旧 message: %w", err)
	}

	// 建立 旧 message_id → 新 event_id 映射，供 user_message 关联。
	eventIDByMessage := make(map[uint]uint, len(messages))
	for _, msg := range messages {
		event, err := buildLegacyEvent(msg)
		if err != nil {
			return err
		}
		if err := dst.Create(event).Error; err != nil {
			return fmt.Errorf("insert notification_event legacy_message_id=%d: %w", msg.ID, err)
		}
		eventIDByMessage[msg.ID] = event.ID
	}

	var userMessages []model.UserMessage
	if err := dst.Order("id").Find(&userMessages).Error; err != nil {
		return fmt.Errorf("读取旧 user_message: %w", err)
	}

	for _, um := range userMessages {
		eventID, ok := eventIDByMessage[um.MessageID]
		if !ok {
			// 孤儿 user_message（对应 message 不存在）跳过。
			continue
		}
		inbox := buildLegacyInbox(um, eventID)
		// 唯一约束 (recipient_user_id, event_id) 保证幂等。
		if err := dst.Clauses(clause.OnConflict{DoNothing: true}).Create(inbox).Error; err != nil {
			return fmt.Errorf("insert notification_inbox legacy_user_message_id=%d: %w", um.ID, err)
		}
	}
	return nil
}

// buildLegacyEvent 把一条旧 message 映射为 notification_event。
func buildLegacyEvent(msg model.Message) (*model.NotificationEvent, error) {
	metadata, err := legacyEventMetadata(msg)
	if err != nil {
		return nil, err
	}

	// 根对象优先取文章，否则回退到 legacy 维度，原始关系保留在 metadata。
	rootType, rootID := "legacy", msg.TypeID
	if msg.ArticleID != nil {
		rootType, rootID = "article", *msg.ArticleID
	}

	actorID := msg.FromUserID
	event := &model.NotificationEvent{
		Type:        legacyEventType(msg.Type),
		ActorUserID: &actorID,
		SourceType:  "legacy",
		SourceID:    msg.TypeID,
		RootType:    rootType,
		RootID:      rootID,
		// 按列宽截断，历史垃圾数据可能远超快照列长度（title 120 / content_excerpt 500）。
		Title:          truncateRunes(strFromPtr(msg.Title), 120),
		ContentExcerpt: truncateRunes(strFromPtr(msg.Content), 500),
		MetadataJSON:   metadata,
		DispatchStatus: "done", // 历史数据视为已分发
		NextProcessAt:  msg.CreatedAt,
	}
	event.CreatedAt = msg.CreatedAt
	event.UpdatedAt = msg.UpdatedAt
	return event, nil
}

// buildLegacyInbox 把一条旧 user_message 映射为 notification_inbox。
func buildLegacyInbox(um model.UserMessage, eventID uint) *model.NotificationInbox {
	inbox := &model.NotificationInbox{
		EventID:         eventID,
		RecipientUserID: um.UserID,
		IsRead:          um.IsRead,
		DeliveredAt:     um.CreatedAt,
	}
	if um.IsRead {
		readAt := um.UpdatedAt
		inbox.ReadAt = &readAt
	}
	inbox.CreatedAt = um.CreatedAt
	inbox.UpdatedAt = um.UpdatedAt
	return inbox
}

// legacyEventMetadata 构造迁移事件的 metadata_json，保留可追溯的旧字段。
func legacyEventMetadata(msg model.Message) (*string, error) {
	meta := map[string]any{
		"legacy_message_id": msg.ID,
		"legacy_type":       msg.Type,
		"legacy_type_id":    msg.TypeID,
	}
	if msg.CommentID != nil {
		meta["legacy_comment_id"] = *msg.CommentID
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("编码 legacy metadata message_id=%d: %w", msg.ID, err)
	}
	value := string(encoded)
	return &value, nil
}

// strFromPtr 解引用字符串指针，nil 返回空串。
func strFromPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// truncateRunes 按 rune 截断字符串到 max 长度，避免在多字节字符中间截断。
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
