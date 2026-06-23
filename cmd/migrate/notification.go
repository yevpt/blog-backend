package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type legacyMessage struct {
	ID         uint
	Title      *string
	Content    *string
	Type       string
	TypeID     uint
	FromUserID uint
	ArticleID  *uint
	CommentID  *uint
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type legacyUserMessage struct {
	ID        uint
	UserID    uint
	MessageID uint
	IsRead    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type legacyNotificationRefs struct {
	comments map[uint]legacyCommentRef
	replies  map[uint]legacyReplyRef
}

type legacyCommentRef struct {
	sourceType string
	sourceID   uint
	rootType   string
	rootID     uint
}

type legacyReplyRef struct {
	sourceType string
	sourceID   uint
	rootType   string
	rootID     uint
}

// legacyReplyQuote 迁移 reply 通知时写入 metadata 的被回复人、父评论与引用摘要。
type legacyReplyQuote struct {
	toUserID      uint
	commentID     uint
	quotedExcerpt string
}

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
	case "comment_like":
		return "comment_liked"
	case "comment_reply":
		return "reply_created"
	case "comment_reply_like":
		return "reply_liked"
	case "guestBook":
		return "guestbook_created"
	case "guestBook_like":
		return "guestbook_liked"
	case "guestBook_reply":
		return "reply_created"
	case "guestBook_reply_like":
		return "reply_liked"
	case "system":
		return "system_notice"
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

// migrateNotifications 把源库 message/user_messages 直接转换为 v2 通知事件与收件箱。
//
// 旧 message 映射为 notification_event（type 规范化，原始信息存入 metadata_json）；
// 旧 user_messages 通过 message_id 关联到事件后映射为 notification_inbox，孤儿记录跳过。
// 文章、碎言、评论、回复、点赞等源头在目标库已不存在时，整条旧消息及其收件箱一并跳过。
// 收件箱唯一约束保证重复执行不产生重复投递。
func migrateNotifications(src *sql.DB, dst *gorm.DB) error {
	messages, err := listLegacyMessages(src)
	if err != nil {
		return err
	}
	userMessages, err := listLegacyUserMessages(src)
	if err != nil {
		return err
	}
	refs, err := buildLegacyNotificationRefs(src)
	if err != nil {
		return err
	}
	existence, err := loadNotificationExistence(dst)
	if err != nil {
		return err
	}
	replyQuotes, err := loadLegacyReplyQuotes(dst)
	if err != nil {
		return err
	}

	// 回填可重复执行：先清空 v2 表，保证多次运行（含上次部分失败）后结果一致。
	if err := dst.Exec("DELETE FROM `notification_inbox`").Error; err != nil {
		return fmt.Errorf("清空 notification_inbox: %w", err)
	}
	if err := dst.Exec("DELETE FROM `notification_event`").Error; err != nil {
		return fmt.Errorf("清空 notification_event: %w", err)
	}

	// 建立 旧 message_id → 新 event_id 映射，供 user_messages 关联。
	eventIDByMessage := make(map[uint]uint, len(messages))
	legacyNoticeTypes := make(map[string]int)
	skippedMissingSource := 0
	for _, msg := range messages {
		if !legacyNotificationTargetValid(msg, refs, existence) {
			skippedMissingSource++
			continue
		}
		event, err := buildLegacyEvent(msg, refs, replyQuotes)
		if err != nil {
			return err
		}
		if event.Type == "legacy_notice" {
			legacyNoticeTypes[msg.Type]++
		}
		if err := dst.Create(event).Error; err != nil {
			return fmt.Errorf("insert notification_event legacy_message_id=%d: %w", msg.ID, err)
		}
		eventIDByMessage[msg.ID] = event.ID
	}
	if skippedMissingSource > 0 {
		log.Printf("  跳过源头已缺失的旧消息 %d 条", skippedMissingSource)
	}
	logLegacyNoticeTypes(legacyNoticeTypes)

	for _, um := range userMessages {
		eventID, ok := eventIDByMessage[um.MessageID]
		if !ok {
			// 孤儿 user_messages（对应 message 不存在）跳过。
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

func listLegacyMessages(src *sql.DB) ([]legacyMessage, error) {
	rows, err := src.Query(`
		SELECT ID, title, content, type, type_id, from_id, post_id, comment_id, date_create
		FROM message ORDER BY ID`)
	if err != nil {
		return nil, fmt.Errorf("读取旧 message: %w", err)
	}
	defer rows.Close()

	var messages []legacyMessage
	for rows.Next() {
		var (
			msg        legacyMessage
			title      sql.NullString
			content    sql.NullString
			msgType    sql.NullString
			postID     sql.NullInt64
			commentID  sql.NullInt64
			dateCreate sql.NullTime
		)
		if err := rows.Scan(&msg.ID, &title, &content, &msgType, &msg.TypeID, &msg.FromUserID, &postID, &commentID, &dateCreate); err != nil {
			return nil, err
		}
		msg.Title = nullStr(title)
		msg.Content = nullStr(content)
		if msgType.Valid {
			msg.Type = msgType.String
		}
		msg.ArticleID = nullUint(postID)
		msg.CommentID = nullUint(commentID)
		if dateCreate.Valid {
			msg.CreatedAt = dateCreate.Time
			msg.UpdatedAt = dateCreate.Time
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func listLegacyUserMessages(src *sql.DB) ([]legacyUserMessage, error) {
	rows, err := src.Query(`
		SELECT um.ID, um.user_id, um.message_id, um.read_status, um.date_create
		FROM user_messages um
		JOIN message m ON m.ID = um.message_id
		ORDER BY um.ID`)
	if err != nil {
		return nil, fmt.Errorf("读取旧 user_messages: %w", err)
	}
	defer rows.Close()

	var userMessages []legacyUserMessage
	for rows.Next() {
		var (
			um         legacyUserMessage
			readStatus sql.NullString
			dateCreate sql.NullTime
		)
		if err := rows.Scan(&um.ID, &um.UserID, &um.MessageID, &readStatus, &dateCreate); err != nil {
			return nil, err
		}
		um.IsRead = readStatus.Valid && readStatus.String == "01"
		if dateCreate.Valid {
			um.CreatedAt = dateCreate.Time
			um.UpdatedAt = dateCreate.Time
		}
		userMessages = append(userMessages, um)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return userMessages, nil
}

func buildLegacyNotificationRefs(src *sql.DB) (legacyNotificationRefs, error) {
	refs := legacyNotificationRefs{
		comments: make(map[uint]legacyCommentRef),
		replies:  make(map[uint]legacyReplyRef),
	}
	if err := loadLegacyCommentRefs(src, refs.comments); err != nil {
		return legacyNotificationRefs{}, err
	}
	if err := loadLegacyReplyRefs(src, refs.replies); err != nil {
		return legacyNotificationRefs{}, err
	}
	return refs, nil
}

func loadLegacyCommentRefs(src *sql.DB, refs map[uint]legacyCommentRef) error {
	rows, err := src.Query("SELECT ID, type, owner_id FROM comment ORDER BY ID")
	if err != nil {
		return fmt.Errorf("构建旧 comment 通知引用: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id      uint
			ctype   string
			ownerID uint
		)
		if err := rows.Scan(&id, &ctype, &ownerID); err != nil {
			return err
		}
		switch ctype {
		case "post":
			refs[id] = legacyCommentRef{sourceType: "comment", sourceID: id, rootType: "article", rootID: ownerID}
		case "say":
			refs[id] = legacyCommentRef{sourceType: "comment", sourceID: id, rootType: "moment", rootID: ownerID}
		case "guestBook":
			refs[id] = legacyCommentRef{sourceType: "guestbook", sourceID: id, rootType: "guestbook", rootID: id}
		}
	}
	return rows.Err()
}

func loadLegacyReplyRefs(src *sql.DB, refs map[uint]legacyReplyRef) error {
	rows, err := src.Query(`
		SELECT cr.ID, cr.comment_id, c.type
		FROM comment_reply cr
		JOIN comment c ON c.ID = cr.comment_id
		ORDER BY cr.ID`)
	if err != nil {
		return fmt.Errorf("构建旧 comment_reply 通知引用: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id        uint
			commentID uint
			ctype     string
		)
		if err := rows.Scan(&id, &commentID, &ctype); err != nil {
			return err
		}
		switch ctype {
		case "post":
			refs[id] = legacyReplyRef{sourceType: "reply", sourceID: id, rootType: "article", rootID: commentID}
		case "say":
			refs[id] = legacyReplyRef{sourceType: "reply", sourceID: id, rootType: "moment", rootID: commentID}
		case "guestBook":
			refs[id] = legacyReplyRef{sourceType: "reply", sourceID: id, rootType: "guestbook", rootID: commentID}
		}
	}
	return rows.Err()
}

// buildLegacyEvent 把一条旧 message 映射为 notification_event。
func buildLegacyEvent(msg legacyMessage, refs legacyNotificationRefs, replyQuotes map[uint]legacyReplyQuote) (*model.NotificationEvent, error) {
	sourceType, sourceID, rootType, rootID := legacyEventRelation(msg, refs)
	eventType := legacyEventTypeForMessage(msg, sourceType)
	metadata, err := legacyEventMetadata(msg, eventType, replyQuotes)
	if err != nil {
		return nil, err
	}

	actorID := msg.FromUserID
	event := &model.NotificationEvent{
		Type:        eventType,
		ActorUserID: &actorID,
		SourceType:  sourceType,
		SourceID:    sourceID,
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

func legacyEventTypeForMessage(msg legacyMessage, sourceType string) string {
	if msg.Type == "comment_like" && sourceType == "guestbook" {
		return "guestbook_liked"
	}
	return legacyEventType(msg.Type)
}

func legacyEventRelation(msg legacyMessage, refs legacyNotificationRefs) (string, uint, string, uint) {
	switch msg.Type {
	case "system":
		return "system", msg.TypeID, "system", msg.TypeID
	case "post_like", "article_like":
		id := msg.TypeID
		if msg.ArticleID != nil {
			id = *msg.ArticleID
		}
		return "article", id, "article", id
	case "say_like", "moment_like":
		return "moment", msg.TypeID, "moment", msg.TypeID
	case "comment", "moment_comment", "say", "comment_like":
		if ref, ok := refs.comments[msg.TypeID]; ok {
			return ref.sourceType, ref.sourceID, ref.rootType, ref.rootID
		}
		if msg.CommentID != nil {
			if ref, ok := refs.comments[*msg.CommentID]; ok {
				return ref.sourceType, ref.sourceID, ref.rootType, ref.rootID
			}
		}
		if msg.ArticleID != nil {
			return "comment", msg.TypeID, "article", *msg.ArticleID
		}
	case "guestBook", "guestBook_like":
		if ref, ok := refs.comments[msg.TypeID]; ok {
			return ref.sourceType, ref.sourceID, ref.rootType, ref.rootID
		}
		return "guestbook", msg.TypeID, "guestbook", msg.TypeID
	case "comment_reply", "guestBook_reply", "comment_reply_like", "guestBook_reply_like":
		if ref, ok := refs.replies[msg.TypeID]; ok {
			return ref.sourceType, ref.sourceID, ref.rootType, ref.rootID
		}
		if msg.CommentID != nil {
			if ref, ok := refs.comments[*msg.CommentID]; ok {
				return "reply", msg.TypeID, ref.rootType, ref.sourceID
			}
		}
		if msg.ArticleID != nil && msg.CommentID != nil {
			return "reply", msg.TypeID, "article", *msg.CommentID
		}
		if msg.CommentID != nil && (msg.Type == "guestBook_reply" || msg.Type == "guestBook_reply_like") {
			return "reply", msg.TypeID, "guestbook", *msg.CommentID
		}
	}
	return "legacy", msg.TypeID, "legacy", msg.TypeID
}

func loadLegacyReplyQuotes(dst *gorm.DB) (map[uint]legacyReplyQuote, error) {
	quotes := make(map[uint]legacyReplyQuote)
	tables := []struct {
		replyTable   string
		commentTable string
	}{
		{replyTable: "article_comment_reply", commentTable: "article_comment"},
		{replyTable: "moment_comment_reply", commentTable: "moment_comment"},
		{replyTable: "guestbook_reply", commentTable: "guestbook"},
	}
	for _, table := range tables {
		if err := appendLegacyReplyQuotes(dst, table.replyTable, table.commentTable, quotes); err != nil {
			return nil, err
		}
	}
	return quotes, nil
}

func appendLegacyReplyQuotes(dst *gorm.DB, replyTable, commentTable string, quotes map[uint]legacyReplyQuote) error {
	commentContents, err := loadLegacyCommentContents(dst, commentTable)
	if err != nil {
		return err
	}

	var rows []struct {
		ID            uint
		CommentID     uint
		ToUserID      uint
		ParentReplyID uint
		Content       string
	}
	if err := dst.Table(replyTable).
		Select("id, comment_id, to_user_id, parent_reply_id, content").
		Find(&rows).Error; err != nil {
		return fmt.Errorf("读取 %s: %w", replyTable, err)
	}

	replyContents := make(map[uint]string, len(rows))
	for _, row := range rows {
		replyContents[row.ID] = row.Content
	}
	for _, row := range rows {
		quoted := commentContents[row.CommentID]
		if row.ParentReplyID != 0 {
			if parent, ok := replyContents[row.ParentReplyID]; ok {
				quoted = parent
			}
		}
		quotes[row.ID] = legacyReplyQuote{
			toUserID:      row.ToUserID,
			commentID:     row.CommentID,
			quotedExcerpt: strings.TrimSpace(quoted),
		}
	}
	return nil
}

func loadLegacyCommentContents(dst *gorm.DB, commentTable string) (map[uint]string, error) {
	var rows []struct {
		ID      uint
		Content string
	}
	if err := dst.Table(commentTable).Select("id, content").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取 %s: %w", commentTable, err)
	}
	contents := make(map[uint]string, len(rows))
	for _, row := range rows {
		contents[row.ID] = row.Content
	}
	return contents, nil
}

// notificationExistence 缓存目标库中通知可引用的业务对象 ID，供迁移与孤儿清理复用。
type notificationExistence struct {
	articles            map[uint]struct{}
	moments             map[uint]struct{}
	guestbooks          map[uint]struct{}
	articleComments     map[uint]struct{}
	momentComments      map[uint]struct{}
	articleCommentReply map[uint]struct{}
	momentCommentReply  map[uint]struct{}
	guestbookReplies    map[uint]struct{}
}

func loadNotificationExistence(dst *gorm.DB) (notificationExistence, error) {
	loadIDs := func(table string) (map[uint]struct{}, error) {
		var ids []uint
		if err := dst.Table(table).Pluck("id", &ids).Error; err != nil {
			return nil, fmt.Errorf("读取 %s.id: %w", table, err)
		}
		set := make(map[uint]struct{}, len(ids))
		for _, id := range ids {
			set[id] = struct{}{}
		}
		return set, nil
	}

	articles, err := loadIDs("article")
	if err != nil {
		return notificationExistence{}, err
	}
	moments, err := loadIDs("moment")
	if err != nil {
		return notificationExistence{}, err
	}
	guestbooks, err := loadIDs("guestbook")
	if err != nil {
		return notificationExistence{}, err
	}
	articleComments, err := loadIDs("article_comment")
	if err != nil {
		return notificationExistence{}, err
	}
	momentComments, err := loadIDs("moment_comment")
	if err != nil {
		return notificationExistence{}, err
	}
	articleCommentReply, err := loadIDs("article_comment_reply")
	if err != nil {
		return notificationExistence{}, err
	}
	momentCommentReply, err := loadIDs("moment_comment_reply")
	if err != nil {
		return notificationExistence{}, err
	}
	guestbookReplies, err := loadIDs("guestbook_reply")
	if err != nil {
		return notificationExistence{}, err
	}

	return notificationExistence{
		articles:            articles,
		moments:             moments,
		guestbooks:          guestbooks,
		articleComments:     articleComments,
		momentComments:      momentComments,
		articleCommentReply: articleCommentReply,
		momentCommentReply:  momentCommentReply,
		guestbookReplies:    guestbookReplies,
	}, nil
}

func (e notificationExistence) has(set map[uint]struct{}, id uint) bool {
	if id == 0 {
		return false
	}
	_, ok := set[id]
	return ok
}

// isMappableLegacyMessageType 判断旧 message.type 是否本应映射到具体业务对象。
func isMappableLegacyMessageType(oldType string) bool {
	switch oldType {
	case "post_like", "article_like", "say_like", "moment_like",
		"comment", "moment_comment", "say", "comment_like",
		"comment_reply", "comment_reply_like",
		"guestBook", "guestBook_like", "guestBook_reply", "guestBook_reply_like":
		return true
	default:
		return false
	}
}

// legacyNotificationTargetValid 判断旧消息引用的业务对象在目标库是否仍存在。
func legacyNotificationTargetValid(msg legacyMessage, refs legacyNotificationRefs, existence notificationExistence) bool {
	if msg.Type == "system" {
		return true
	}
	sourceType, sourceID, rootType, rootID := legacyEventRelation(msg, refs)
	if sourceType == "legacy" {
		// 可映射类型因源头缺失回退 legacy，视为孤儿；真正未知类型保留。
		return !isMappableLegacyMessageType(msg.Type)
	}
	return notificationTargetExists(sourceType, sourceID, rootType, rootID, existence)
}

func notificationTargetExists(sourceType string, sourceID uint, rootType string, rootID uint, existence notificationExistence) bool {
	switch sourceType {
	case "system":
		return true
	case "article":
		return existence.has(existence.articles, sourceID)
	case "moment":
		return existence.has(existence.moments, sourceID)
	case "guestbook":
		return existence.has(existence.guestbooks, sourceID)
	case "comment":
		switch rootType {
		case "article":
			return existence.has(existence.articleComments, sourceID) && existence.has(existence.articles, rootID)
		case "moment":
			return existence.has(existence.momentComments, sourceID) && existence.has(existence.moments, rootID)
		case "guestbook":
			return existence.has(existence.guestbooks, sourceID)
		default:
			return false
		}
	case "reply":
		switch rootType {
		case "article":
			return existence.has(existence.articleCommentReply, sourceID) && existence.has(existence.articleComments, rootID)
		case "moment":
			return existence.has(existence.momentCommentReply, sourceID) && existence.has(existence.momentComments, rootID)
		case "guestbook":
			return existence.has(existence.guestbookReplies, sourceID) && existence.has(existence.guestbooks, rootID)
		default:
			return false
		}
	default:
		return false
	}
}

// pruneOrphanNotificationEvents 删除源头业务对象已不存在的通知事件（含 legacy 回退事件）。
func pruneOrphanNotificationEvents(dst *gorm.DB) error {
	existence, err := loadNotificationExistence(dst)
	if err != nil {
		return err
	}

	var events []model.NotificationEvent
	if err := dst.Select("id", "type", "source_type", "source_id", "root_type", "root_id", "metadata_json").
		Find(&events).Error; err != nil {
		return fmt.Errorf("读取 notification_event: %w", err)
	}

	orphanIDs := make([]uint, 0)
	for _, event := range events {
		if notificationEventTargetValid(event, existence) {
			continue
		}
		orphanIDs = append(orphanIDs, event.ID)
	}
	if len(orphanIDs) == 0 {
		return nil
	}
	result := dst.Where("id IN ?", orphanIDs).Delete(&model.NotificationEvent{})
	if result.Error != nil {
		return fmt.Errorf("清理 notification_event 源头缺失: %w", result.Error)
	}
	log.Printf("  清理 notification_event 源头缺失: %d 条", result.RowsAffected)
	return nil
}

func notificationEventTargetValid(event model.NotificationEvent, existence notificationExistence) bool {
	if event.Type == "system_notice" || event.SourceType == "system" {
		return true
	}
	if event.SourceType == "legacy" || event.RootType == "legacy" {
		return legacyStoredEventTargetValid(event, existence)
	}
	return notificationTargetExists(event.SourceType, event.SourceID, event.RootType, event.RootID, existence)
}

func legacyStoredEventTargetValid(event model.NotificationEvent, existence notificationExistence) bool {
	msg, ok := legacyMessageFromEventMetadata(event)
	if !ok {
		return event.Type == "legacy_notice"
	}
	if !isMappableLegacyMessageType(msg.Type) {
		return true
	}
	return legacyNotificationTargetValid(msg, legacyNotificationRefs{}, existence)
}

func legacyMessageFromEventMetadata(event model.NotificationEvent) (legacyMessage, bool) {
	if event.MetadataJSON == nil {
		return legacyMessage{}, false
	}
	var meta struct {
		LegacyMessageID uint  `json:"legacy_message_id"`
		LegacyType      string `json:"legacy_type"`
		LegacyTypeID    uint  `json:"legacy_type_id"`
		LegacyCommentID *uint `json:"legacy_comment_id"`
		LegacyPostID    *uint `json:"legacy_post_id"`
	}
	if err := json.Unmarshal([]byte(*event.MetadataJSON), &meta); err != nil || meta.LegacyType == "" {
		return legacyMessage{}, false
	}
	return legacyMessage{
		ID:         meta.LegacyMessageID,
		Type:       meta.LegacyType,
		TypeID:     meta.LegacyTypeID,
		CommentID:  meta.LegacyCommentID,
		ArticleID:  meta.LegacyPostID,
	}, true
}

func logLegacyNoticeTypes(types map[string]int) {
	if len(types) == 0 {
		return
	}
	keys := make([]string, 0, len(types))
	for key := range types {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	log.Printf("  警告: notification_event 仍存在 legacy_notice，请补充旧 message.type 映射：")
	for _, key := range keys {
		log.Printf("    legacy_type=%q count=%d", key, types[key])
	}
}

// buildLegacyInbox 把一条旧 user_messages 映射为 notification_inbox。
func buildLegacyInbox(um legacyUserMessage, eventID uint) *model.NotificationInbox {
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
func legacyEventMetadata(msg legacyMessage, eventType string, replyQuotes map[uint]legacyReplyQuote) (*string, error) {
	meta := map[string]any{
		"legacy_message_id": msg.ID,
		"legacy_type":       msg.Type,
		"legacy_type_id":    msg.TypeID,
	}
	if msg.CommentID != nil {
		meta["legacy_comment_id"] = *msg.CommentID
	}
	if msg.ArticleID != nil {
		meta["legacy_post_id"] = *msg.ArticleID
	}
	if eventType == "reply_created" || eventType == "reply_liked" {
		mergeLegacyReplyMetadata(meta, msg.TypeID, replyQuotes)
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("编码 legacy metadata message_id=%d: %w", msg.ID, err)
	}
	value := string(encoded)
	return &value, nil
}

func mergeLegacyReplyMetadata(meta map[string]any, replyID uint, replyQuotes map[uint]legacyReplyQuote) {
	quote, ok := replyQuotes[replyID]
	if !ok {
		return
	}
	if quote.toUserID != 0 {
		meta["recipient_user_ids"] = []uint{quote.toUserID}
	}
	if quote.commentID != 0 {
		meta["comment_id"] = quote.commentID
	}
	if quote.quotedExcerpt != "" {
		meta["quoted_excerpt"] = quote.quotedExcerpt
	}
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
