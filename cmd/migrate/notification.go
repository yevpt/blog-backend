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
	quotedType    string
	quotedID      uint
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
	commentContents, err := loadLegacyCommentContentIndex(dst)
	if err != nil {
		return err
	}
	snapshots, err := loadLegacyNotificationSnapshots(dst)
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
		event, err := buildLegacyEvent(msg, refs, replyQuotes, commentContents, snapshots)
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
		SELECT cr.ID, cr.comment_id, c.type, c.owner_id
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
			ownerID   uint
		)
		if err := rows.Scan(&id, &commentID, &ctype, &ownerID); err != nil {
			return err
		}
		switch ctype {
		case "post":
			refs[id] = legacyReplyRef{sourceType: "reply", sourceID: id, rootType: "article", rootID: ownerID}
		case "say":
			refs[id] = legacyReplyRef{sourceType: "reply", sourceID: id, rootType: "moment", rootID: ownerID}
		case "guestBook":
			refs[id] = legacyReplyRef{sourceType: "reply", sourceID: id, rootType: "guestbook", rootID: commentID}
		}
	}
	return rows.Err()
}

// buildLegacyEvent 把一条旧 message 映射为 notification_event。
func buildLegacyEvent(
	msg legacyMessage,
	refs legacyNotificationRefs,
	replyQuotes map[uint]legacyReplyQuote,
	commentContents legacyCommentContentIndex,
	snapshots legacyNotificationSnapshotIndex,
) (*model.NotificationEvent, error) {
	sourceType, sourceID, rootType, rootID := legacyEventRelation(msg, refs)
	eventType := legacyEventTypeForMessage(msg, sourceType)
	metadata, err := legacyEventMetadata(msg, eventType, refs, replyQuotes, commentContents, snapshots)
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
				return "reply", msg.TypeID, ref.rootType, ref.rootID
			}
		}
		if msg.ArticleID != nil && msg.CommentID != nil {
			return "reply", msg.TypeID, "article", *msg.ArticleID
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

// legacyCommentContentIndex 按评论所属根对象类型索引评论正文，供 comment_liked 迁移回填。
type legacyCommentContentIndex struct {
	article   map[uint]string
	moment    map[uint]string
	guestbook map[uint]string
}

func loadLegacyCommentContentIndex(dst *gorm.DB) (legacyCommentContentIndex, error) {
	article, err := loadLegacyCommentContents(dst, "article_comment")
	if err != nil {
		return legacyCommentContentIndex{}, err
	}
	moment, err := loadLegacyCommentContents(dst, "moment_comment")
	if err != nil {
		return legacyCommentContentIndex{}, err
	}
	guestbook, err := loadLegacyCommentContents(dst, "guestbook")
	if err != nil {
		return legacyCommentContentIndex{}, err
	}
	return legacyCommentContentIndex{
		article:   article,
		moment:    moment,
		guestbook: guestbook,
	}, nil
}

func (idx legacyCommentContentIndex) content(rootType string, commentID uint) string {
	if commentID == 0 {
		return ""
	}
	switch rootType {
	case "article":
		return strings.TrimSpace(idx.article[commentID])
	case "moment":
		return strings.TrimSpace(idx.moment[commentID])
	case "guestbook":
		return strings.TrimSpace(idx.guestbook[commentID])
	default:
		return ""
	}
}

type legacyNotificationObjectKey struct {
	objectType string
	rootType   string
	id         uint
}

type legacyNotificationSnapshot struct {
	objectType string
	rootType   string
	id         uint
	title      string
	excerpt    string
}

type legacyNotificationSnapshotIndex struct {
	objects map[legacyNotificationObjectKey]legacyNotificationSnapshot
}

func loadLegacyNotificationSnapshots(dst *gorm.DB) (legacyNotificationSnapshotIndex, error) {
	idx := legacyNotificationSnapshotIndex{objects: make(map[legacyNotificationObjectKey]legacyNotificationSnapshot)}
	if err := idx.loadArticles(dst); err != nil {
		return legacyNotificationSnapshotIndex{}, err
	}
	if err := idx.loadMoments(dst); err != nil {
		return legacyNotificationSnapshotIndex{}, err
	}
	if err := idx.loadGuestbooks(dst); err != nil {
		return legacyNotificationSnapshotIndex{}, err
	}
	if err := idx.loadComments(dst, "article_comment", "article"); err != nil {
		return legacyNotificationSnapshotIndex{}, err
	}
	if err := idx.loadComments(dst, "moment_comment", "moment"); err != nil {
		return legacyNotificationSnapshotIndex{}, err
	}
	if err := idx.loadReplies(dst, "article_comment_reply", "article"); err != nil {
		return legacyNotificationSnapshotIndex{}, err
	}
	if err := idx.loadReplies(dst, "moment_comment_reply", "moment"); err != nil {
		return legacyNotificationSnapshotIndex{}, err
	}
	if err := idx.loadReplies(dst, "guestbook_reply", "guestbook"); err != nil {
		return legacyNotificationSnapshotIndex{}, err
	}
	return idx, nil
}

func (idx legacyNotificationSnapshotIndex) add(snapshot legacyNotificationSnapshot) {
	if idx.objects == nil || snapshot.id == 0 || snapshot.objectType == "" {
		return
	}
	snapshot.title = truncateRunes(strings.TrimSpace(snapshot.title), 120)
	snapshot.excerpt = truncateRunes(strings.TrimSpace(snapshot.excerpt), 500)
	idx.objects[legacyNotificationObjectKey{
		objectType: snapshot.objectType,
		rootType:   snapshot.rootType,
		id:         snapshot.id,
	}] = snapshot
}

func (idx legacyNotificationSnapshotIndex) snapshot(objectType string, id uint, rootType string) (legacyNotificationSnapshot, bool) {
	if idx.objects == nil || id == 0 || objectType == "" {
		return legacyNotificationSnapshot{}, false
	}
	if snapshot, ok := idx.objects[legacyNotificationObjectKey{objectType: objectType, rootType: rootType, id: id}]; ok {
		return snapshot, true
	}
	snapshot, ok := idx.objects[legacyNotificationObjectKey{objectType: objectType, id: id}]
	return snapshot, ok
}

func (idx legacyNotificationSnapshotIndex) loadArticles(dst *gorm.DB) error {
	var rows []struct {
		ID           uint
		Title        string
		ShortContent *string
		Content      string
	}
	if err := dst.Table("article").
		Select("id, title, short_content, content").
		Find(&rows).Error; err != nil {
		return fmt.Errorf("读取 article 快照: %w", err)
	}
	for _, row := range rows {
		excerpt := strings.TrimSpace(strFromPtr(row.ShortContent))
		if excerpt == "" {
			excerpt = row.Content
		}
		idx.add(legacyNotificationSnapshot{
			objectType: "article",
			id:         row.ID,
			title:      row.Title,
			excerpt:    excerpt,
		})
	}
	return nil
}

func (idx legacyNotificationSnapshotIndex) loadMoments(dst *gorm.DB) error {
	var rows []struct {
		ID      uint
		Content string
	}
	if err := dst.Table("moment").Select("id, content").Find(&rows).Error; err != nil {
		return fmt.Errorf("读取 moment 快照: %w", err)
	}
	for _, row := range rows {
		idx.add(legacyNotificationSnapshot{
			objectType: "moment",
			id:         row.ID,
			excerpt:    row.Content,
		})
	}
	return nil
}

func (idx legacyNotificationSnapshotIndex) loadGuestbooks(dst *gorm.DB) error {
	var rows []struct {
		ID      uint
		Content string
	}
	if err := dst.Table("guestbook").Select("id, content").Find(&rows).Error; err != nil {
		return fmt.Errorf("读取 guestbook 快照: %w", err)
	}
	for _, row := range rows {
		idx.add(legacyNotificationSnapshot{
			objectType: "guestbook",
			id:         row.ID,
			excerpt:    row.Content,
		})
	}
	return nil
}

func (idx legacyNotificationSnapshotIndex) loadComments(dst *gorm.DB, table string, rootType string) error {
	var rows []struct {
		ID      uint
		Content string
	}
	if err := dst.Table(table).Select("id, content").Find(&rows).Error; err != nil {
		return fmt.Errorf("读取 %s 快照: %w", table, err)
	}
	for _, row := range rows {
		idx.add(legacyNotificationSnapshot{
			objectType: "comment",
			rootType:   rootType,
			id:         row.ID,
			excerpt:    row.Content,
		})
	}
	return nil
}

func (idx legacyNotificationSnapshotIndex) loadReplies(dst *gorm.DB, table string, rootType string) error {
	var rows []struct {
		ID      uint
		Content string
	}
	if err := dst.Table(table).Select("id, content").Find(&rows).Error; err != nil {
		return fmt.Errorf("读取 %s 快照: %w", table, err)
	}
	for _, row := range rows {
		idx.add(legacyNotificationSnapshot{
			objectType: "reply",
			rootType:   rootType,
			id:         row.ID,
			excerpt:    row.Content,
		})
	}
	return nil
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
		quotedType := "comment"
		quotedID := row.CommentID
		if row.ParentReplyID != 0 {
			if parent, ok := replyContents[row.ParentReplyID]; ok {
				quoted = parent
				quotedType = "reply"
				quotedID = row.ParentReplyID
			}
		}
		quotes[row.ID] = legacyReplyQuote{
			toUserID:      row.ToUserID,
			commentID:     row.CommentID,
			quotedType:    quotedType,
			quotedID:      quotedID,
			quotedExcerpt: strings.TrimSpace(quoted),
		}
	}
	return nil
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

func legacyNotificationTargetValid(msg legacyMessage, refs legacyNotificationRefs, existence notificationExistence) bool {
	if msg.Type == "system" {
		return true
	}
	sourceType, sourceID, rootType, rootID := legacyEventRelation(msg, refs)
	if sourceType == "legacy" {
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
			return existence.has(existence.articleCommentReply, sourceID) && existence.has(existence.articles, rootID)
		case "moment":
			return existence.has(existence.momentCommentReply, sourceID) && existence.has(existence.moments, rootID)
		case "guestbook":
			return existence.has(existence.guestbookReplies, sourceID) && existence.has(existence.guestbooks, rootID)
		default:
			return false
		}
	default:
		return false
	}
}

func pruneOrphanNotificationEvents(dst *gorm.DB) (int64, error) {
	existence, err := loadNotificationExistence(dst)
	if err != nil {
		return 0, err
	}

	var events []model.NotificationEvent
	if err := dst.Select("id", "type", "source_type", "source_id", "root_type", "root_id").
		Find(&events).Error; err != nil {
		return 0, fmt.Errorf("读取 notification_event: %w", err)
	}

	orphanIDs := make([]uint, 0)
	for _, event := range events {
		if notificationEventTargetValid(event, existence) {
			continue
		}
		orphanIDs = append(orphanIDs, event.ID)
	}
	if len(orphanIDs) == 0 {
		return 0, nil
	}
	result := dst.Where("id IN ?", orphanIDs).Delete(&model.NotificationEvent{})
	if result.Error != nil {
		return 0, fmt.Errorf("清理 notification_event 源头缺失: %w", result.Error)
	}
	log.Printf("  清理 notification_event 源头缺失: %d 条", result.RowsAffected)
	return result.RowsAffected, nil
}

func notificationEventTargetValid(event model.NotificationEvent, existence notificationExistence) bool {
	if event.Type == "system_notice" || event.SourceType == "system" {
		return true
	}
	if event.SourceType == "legacy" || event.RootType == "legacy" {
		return event.Type == "legacy_notice"
	}
	return notificationTargetExists(event.SourceType, event.SourceID, event.RootType, event.RootID, existence)
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

func refreshNotificationMetadata(dst *gorm.DB) error {
	replyQuotes, err := loadLegacyReplyQuotes(dst)
	if err != nil {
		return err
	}
	commentContents, err := loadLegacyCommentContentIndex(dst)
	if err != nil {
		return err
	}
	snapshots, err := loadLegacyNotificationSnapshots(dst)
	if err != nil {
		return err
	}

	var events []model.NotificationEvent
	if err := dst.Find(&events).Error; err != nil {
		return fmt.Errorf("读取 notification_event: %w", err)
	}
	for _, event := range events {
		metadata, err := legacyEventMetadataFromEvent(event, replyQuotes, commentContents, snapshots)
		if err != nil {
			return err
		}
		if err := dst.Model(&model.NotificationEvent{}).
			Where("id = ?", event.ID).
			Update("metadata_json", metadata).Error; err != nil {
			return fmt.Errorf("刷新 notification_event metadata id=%d: %w", event.ID, err)
		}
	}
	return nil
}

type legacyNotificationActorIndex struct {
	guestbooks       map[uint]uint
	articleComments  map[uint]uint
	momentComments   map[uint]uint
	articleReplies   map[uint]uint
	momentReplies    map[uint]uint
	guestbookReplies map[uint]uint
}

type legacyNotificationActorRow struct {
	ID     uint
	UserID uint
}

func loadLegacyNotificationActorIndex(dst *gorm.DB) (legacyNotificationActorIndex, error) {
	guestbooks, err := loadLegacyNotificationActorMap(dst, "guestbook", "from_user_id")
	if err != nil {
		return legacyNotificationActorIndex{}, err
	}
	articleComments, err := loadLegacyNotificationActorMap(dst, "article_comment", "user_id")
	if err != nil {
		return legacyNotificationActorIndex{}, err
	}
	momentComments, err := loadLegacyNotificationActorMap(dst, "moment_comment", "user_id")
	if err != nil {
		return legacyNotificationActorIndex{}, err
	}
	articleReplies, err := loadLegacyNotificationActorMap(dst, "article_comment_reply", "from_user_id")
	if err != nil {
		return legacyNotificationActorIndex{}, err
	}
	momentReplies, err := loadLegacyNotificationActorMap(dst, "moment_comment_reply", "from_user_id")
	if err != nil {
		return legacyNotificationActorIndex{}, err
	}
	guestbookReplies, err := loadLegacyNotificationActorMap(dst, "guestbook_reply", "from_user_id")
	if err != nil {
		return legacyNotificationActorIndex{}, err
	}

	return legacyNotificationActorIndex{
		guestbooks:       guestbooks,
		articleComments:  articleComments,
		momentComments:   momentComments,
		articleReplies:   articleReplies,
		momentReplies:    momentReplies,
		guestbookReplies: guestbookReplies,
	}, nil
}

func loadLegacyNotificationActorMap(dst *gorm.DB, tableName string, userColumn string) (map[uint]uint, error) {
	var rows []legacyNotificationActorRow
	query := fmt.Sprintf("SELECT id, %s AS user_id FROM %s", userColumn, tableName)
	if err := dst.Raw(query).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取通知 actor 索引 %s: %w", tableName, err)
	}
	result := make(map[uint]uint, len(rows))
	for _, row := range rows {
		if row.ID == 0 || row.UserID == 0 {
			continue
		}
		result[row.ID] = row.UserID
	}
	return result, nil
}

func refreshNotificationActors(dst *gorm.DB) error {
	actors, err := loadLegacyNotificationActorIndex(dst)
	if err != nil {
		return err
	}

	var events []model.NotificationEvent
	if err := dst.
		Where("type IN ?", []string{"guestbook_created", "comment_created", "reply_created"}).
		Find(&events).Error; err != nil {
		return fmt.Errorf("读取 notification_event actor: %w", err)
	}
	for _, event := range events {
		actorID := legacyActorUserIDForEvent(event, actors)
		if actorID == nil {
			continue
		}
		if err := dst.Model(&model.NotificationEvent{}).
			Where("id = ?", event.ID).
			Update("actor_user_id", *actorID).Error; err != nil {
			return fmt.Errorf("刷新 notification_event actor id=%d: %w", event.ID, err)
		}
	}
	return nil
}

func legacyActorUserIDForEvent(event model.NotificationEvent, actors legacyNotificationActorIndex) *uint {
	var actorID uint
	switch event.Type {
	case "guestbook_created":
		if event.SourceType != "guestbook" {
			return nil
		}
		actorID = actors.guestbooks[event.SourceID]
	case "comment_created":
		actorID = legacyCommentActorUserID(event, actors)
	case "reply_created":
		actorID = legacyReplyActorUserID(event, actors)
	default:
		return nil
	}
	if actorID == 0 {
		return nil
	}
	return &actorID
}

func legacyCommentActorUserID(event model.NotificationEvent, actors legacyNotificationActorIndex) uint {
	switch event.RootType {
	case "article":
		return actors.articleComments[event.SourceID]
	case "moment":
		return actors.momentComments[event.SourceID]
	case "guestbook":
		return actors.guestbooks[event.SourceID]
	default:
		return 0
	}
}

func legacyReplyActorUserID(event model.NotificationEvent, actors legacyNotificationActorIndex) uint {
	switch event.RootType {
	case "article":
		return actors.articleReplies[event.SourceID]
	case "moment":
		return actors.momentReplies[event.SourceID]
	case "guestbook":
		return actors.guestbookReplies[event.SourceID]
	default:
		return 0
	}
}

// legacyEventMetadata 构造迁移事件的 metadata_json，保留可追溯的旧字段。
func legacyEventMetadata(
	msg legacyMessage,
	eventType string,
	refs legacyNotificationRefs,
	replyQuotes map[uint]legacyReplyQuote,
	commentContents legacyCommentContentIndex,
	snapshots legacyNotificationSnapshotIndex,
) (*string, error) {
	meta := map[string]any{}
	sourceType, sourceID, rootType, rootID := legacyEventRelation(msg, refs)
	if sourceSnapshot := legacySourceSnapshot(sourceType, sourceID, rootType, snapshots); sourceSnapshot != nil {
		meta["source_snapshot"] = sourceSnapshot
	}
	if rootSnapshot := legacyRootSnapshot(sourceType, rootType, rootID, snapshots); rootSnapshot != nil {
		meta["root_snapshot"] = rootSnapshot
	}
	if eventType == "reply_created" || eventType == "reply_liked" {
		mergeLegacyReplyMetadata(meta, sourceID, replyQuotes)
	}
	if eventType == "comment_liked" {
		mergeLegacyCommentLikedMetadata(meta, msg, refs, commentContents)
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("编码 legacy metadata message_id=%d: %w", msg.ID, err)
	}
	value := string(encoded)
	return &value, nil
}

func legacyEventMetadataFromEvent(
	event model.NotificationEvent,
	replyQuotes map[uint]legacyReplyQuote,
	commentContents legacyCommentContentIndex,
	snapshots legacyNotificationSnapshotIndex,
) (*string, error) {
	meta := map[string]any{}
	if sourceSnapshot := legacySourceSnapshot(event.SourceType, event.SourceID, event.RootType, snapshots); sourceSnapshot != nil {
		meta["source_snapshot"] = sourceSnapshot
	}
	if rootSnapshot := legacyRootSnapshot(event.SourceType, event.RootType, event.RootID, snapshots); rootSnapshot != nil {
		meta["root_snapshot"] = rootSnapshot
	}
	if event.Type == "reply_created" || event.Type == "reply_liked" {
		mergeLegacyReplyMetadata(meta, event.SourceID, replyQuotes)
	}
	if event.Type == "comment_liked" {
		if excerpt := commentContents.content(event.RootType, event.SourceID); excerpt != "" {
			meta["source_snapshot"] = legacySnapshot("comment", event.SourceID, nil, &excerpt)
		}
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("编码 notification_event metadata event_id=%d: %w", event.ID, err)
	}
	value := string(encoded)
	return &value, nil
}

func legacySourceSnapshot(sourceType string, sourceID uint, rootType string, snapshots legacyNotificationSnapshotIndex) map[string]any {
	if snapshot, ok := snapshots.snapshot(sourceType, sourceID, rootType); ok {
		return legacySnapshot(snapshot.objectType, snapshot.id, &snapshot.title, &snapshot.excerpt)
	}
	return legacySnapshot(sourceType, sourceID, nil, nil)
}

func legacyRootSnapshot(sourceType string, rootType string, rootID uint, snapshots legacyNotificationSnapshotIndex) map[string]any {
	if snapshot, ok := snapshots.snapshot(rootType, rootID, ""); ok {
		return legacySnapshot(snapshot.objectType, snapshot.id, &snapshot.title, &snapshot.excerpt)
	}
	return legacySnapshot(rootType, rootID, nil, nil)
}

func mergeLegacyCommentLikedMetadata(meta map[string]any, msg legacyMessage, refs legacyNotificationRefs, contents legacyCommentContentIndex) {
	commentID := msg.TypeID
	rootType := ""
	if ref, ok := refs.comments[msg.TypeID]; ok {
		commentID = ref.sourceID
		rootType = ref.rootType
	} else if msg.ArticleID != nil {
		rootType = "article"
	}
	if excerpt := contents.content(rootType, commentID); excerpt != "" {
		meta["source_snapshot"] = legacySnapshot("comment", commentID, nil, &excerpt)
	}
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
		objectType := quote.quotedType
		if objectType == "" {
			objectType = "comment"
		}
		quotedID := quote.quotedID
		if quotedID == 0 {
			quotedID = quote.commentID
		}
		meta["quote_snapshot"] = legacySnapshot(objectType, quotedID, nil, &quote.quotedExcerpt)
	}
}

func legacySnapshot(objectType string, id uint, title *string, excerpt *string) map[string]any {
	objectType = strings.TrimSpace(objectType)
	titleValue := strings.TrimSpace(strFromPtr(title))
	excerptValue := strings.TrimSpace(strFromPtr(excerpt))
	if objectType == "" && id == 0 && titleValue == "" && excerptValue == "" {
		return nil
	}
	snapshot := map[string]any{}
	if objectType != "" {
		snapshot["type"] = objectType
	}
	if id != 0 {
		snapshot["id"] = id
	}
	if titleValue != "" {
		snapshot["title"] = titleValue
	}
	if excerptValue != "" {
		snapshot["excerpt"] = excerptValue
	}
	return snapshot
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
