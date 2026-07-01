package moderation

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

const notificationSnapshotLimit = 500

func (r *repository) LoadReviewNotificationContext(ctx context.Context, ref SubjectRef) (ReviewNotificationContext, error) {
	result := ReviewNotificationContext{ContentType: ref.Type}
	if r == nil || r.db == nil {
		return result, nil
	}
	db := r.db.WithContext(ctx)
	recipientID, err := loadInteractionRecipient(db, ref)
	if err != nil {
		return result, err
	}
	result.InteractionRecipientUserID = recipientID
	switch ref.Type {
	case SubjectMoment:
		return result, nil
	case SubjectGuestbook:
		result.RootSnapshot, err = loadGuestbookSnapshot(db, ref.ID)
	case SubjectArticleComment:
		result.RootSnapshot, err = loadArticleSnapshot(db, ref.RootID)
	case SubjectMomentComment:
		result.RootSnapshot, err = loadMomentSnapshot(db, ref.RootID)
	case SubjectArticleCommentReply:
		result.CommentID = uint64Pointer(ref.RootID)
		result.RootSnapshot, result.QuoteSnapshot, err = loadArticleReplyContext(db, ref)
	case SubjectMomentCommentReply:
		result.CommentID = uint64Pointer(ref.RootID)
		result.RootSnapshot, result.QuoteSnapshot, err = loadMomentReplyContext(db, ref)
	case SubjectGuestbookReply:
		result.CommentID = uint64Pointer(ref.RootID)
		result.RootSnapshot, result.QuoteSnapshot, err = loadGuestbookReplyContext(db, ref)
	default:
		return result, nil
	}
	if err != nil && !errors.Is(err, ErrSubjectNotFound) {
		return result, err
	}
	return result, nil
}

func loadInteractionRecipient(db *gorm.DB, ref SubjectRef) (uint64, error) {
	if ref.Type == SubjectGuestbook {
		return ref.RootID, nil
	}
	var row struct {
		UserID     uint
		FromUserID uint
		ToUserID   uint
	}
	var query *gorm.DB
	switch ref.Type {
	case SubjectMoment:
		return 0, nil
	case SubjectArticleComment:
		query = db.Model(&model.Article{}).Select("user_id").Where("id = ?", ref.RootID).Take(&row)
	case SubjectMomentComment:
		query = db.Model(&model.Moment{}).Select("user_id").Where("id = ?", ref.RootID).Take(&row)
	case SubjectArticleCommentReply, SubjectMomentCommentReply, SubjectGuestbookReply:
		return loadReplyRecipient(db, ref)
	default:
		return 0, nil
	}
	if query.Error != nil {
		return 0, subjectError(query.Error)
	}
	return uint64(row.UserID), nil
}

func loadReplyRecipient(db *gorm.DB, ref SubjectRef) (uint64, error) {
	var row struct {
		UserID     uint
		FromUserID uint
		ToUserID   uint
	}
	var query *gorm.DB
	if ref.ID != 0 {
		switch ref.Type {
		case SubjectArticleCommentReply:
			query = db.Model(&model.ArticleCommentReply{}).Select("to_user_id").Where("id = ?", ref.ID).Take(&row)
		case SubjectMomentCommentReply:
			query = db.Model(&model.MomentCommentReply{}).Select("to_user_id").Where("id = ?", ref.ID).Take(&row)
		case SubjectGuestbookReply:
			query = db.Model(&model.GuestbookReply{}).Select("to_user_id").Where("id = ?", ref.ID).Take(&row)
		}
		if query.Error != nil {
			return 0, subjectError(query.Error)
		}
		return uint64(row.ToUserID), nil
	}
	if ref.ParentID == nil {
		return 0, ErrInvalidCommand
	}
	if *ref.ParentID == 0 {
		switch ref.Type {
		case SubjectArticleCommentReply:
			query = db.Model(&model.ArticleComment{}).Select("user_id").Where("id = ?", ref.RootID).Take(&row)
		case SubjectMomentCommentReply:
			query = db.Model(&model.MomentComment{}).Select("user_id").Where("id = ?", ref.RootID).Take(&row)
		case SubjectGuestbookReply:
			query = db.Model(&model.Guestbook{}).Select("from_user_id").Where("id = ?", ref.RootID).Take(&row)
		}
		if query.Error != nil {
			return 0, subjectError(query.Error)
		}
		if ref.Type == SubjectGuestbookReply {
			return uint64(row.FromUserID), nil
		}
		return uint64(row.UserID), nil
	}
	switch ref.Type {
	case SubjectArticleCommentReply:
		query = db.Model(&model.ArticleCommentReply{}).Select("from_user_id").Where("id = ? AND comment_id = ?", *ref.ParentID, ref.RootID).Take(&row)
	case SubjectMomentCommentReply:
		query = db.Model(&model.MomentCommentReply{}).Select("from_user_id").Where("id = ? AND comment_id = ?", *ref.ParentID, ref.RootID).Take(&row)
	case SubjectGuestbookReply:
		query = db.Model(&model.GuestbookReply{}).Select("from_user_id").Where("id = ? AND comment_id = ?", *ref.ParentID, ref.RootID).Take(&row)
	}
	if query.Error != nil {
		return 0, subjectError(query.Error)
	}
	return uint64(row.FromUserID), nil
}

func loadArticleSnapshot(db *gorm.DB, articleID uint64) (*NotificationSnapshot, error) {
	if articleID == 0 {
		return nil, nil
	}
	var row model.Article
	if err := db.Select("id", "title", "short_content", "content").First(&row, articleID).Error; err != nil {
		return nil, subjectError(err)
	}
	excerpt := articleExcerpt(row)
	return notificationSnapshot("article", uint64(row.ID), row.Title, excerpt), nil
}

func loadMomentSnapshot(db *gorm.DB, momentID uint64) (*NotificationSnapshot, error) {
	if momentID == 0 {
		return nil, nil
	}
	var row model.Moment
	if err := db.Select("id", "content").First(&row, momentID).Error; err != nil {
		return nil, subjectError(err)
	}
	return notificationSnapshot("moment", uint64(row.ID), "", row.Content), nil
}

func loadGuestbookSnapshot(db *gorm.DB, messageID uint64) (*NotificationSnapshot, error) {
	if messageID == 0 {
		return nil, nil
	}
	var row model.Guestbook
	if err := db.Select("id", "content").First(&row, messageID).Error; err != nil {
		return nil, subjectError(err)
	}
	return notificationSnapshot("guestbook", uint64(row.ID), "", row.Content), nil
}

func loadArticleReplyContext(db *gorm.DB, ref SubjectRef) (*NotificationSnapshot, *NotificationSnapshot, error) {
	var comment model.ArticleComment
	if err := db.Select("id", "article_id", "content").First(&comment, ref.RootID).Error; err != nil {
		return nil, nil, subjectError(err)
	}
	root, err := loadArticleSnapshot(db, uint64(comment.ArticleID))
	if err != nil {
		return nil, nil, err
	}
	quote, err := loadReplyQuote(db, ref, comment.Content, loadArticleCommentReplyContent)
	return root, quote, err
}

func loadMomentReplyContext(db *gorm.DB, ref SubjectRef) (*NotificationSnapshot, *NotificationSnapshot, error) {
	var comment model.MomentComment
	if err := db.Select("id", "moment_id", "content").First(&comment, ref.RootID).Error; err != nil {
		return nil, nil, subjectError(err)
	}
	root, err := loadMomentSnapshot(db, uint64(comment.MomentID))
	if err != nil {
		return nil, nil, err
	}
	quote, err := loadReplyQuote(db, ref, comment.Content, loadMomentCommentReplyContent)
	return root, quote, err
}

func loadGuestbookReplyContext(db *gorm.DB, ref SubjectRef) (*NotificationSnapshot, *NotificationSnapshot, error) {
	root, err := loadGuestbookSnapshot(db, ref.RootID)
	if err != nil {
		return nil, nil, err
	}
	var message model.Guestbook
	if err := db.Select("content").First(&message, ref.RootID).Error; err != nil {
		return nil, nil, subjectError(err)
	}
	quote, err := loadReplyQuote(db, ref, message.Content, loadGuestbookReplyContent)
	return root, quote, err
}

type replyContentLoader func(db *gorm.DB, replyID uint64) (string, error)

func loadReplyQuote(db *gorm.DB, ref SubjectRef, commentContent string, loadReply replyContentLoader) (*NotificationSnapshot, error) {
	if ref.ParentID == nil || *ref.ParentID == 0 {
		return notificationSnapshot("comment", ref.RootID, "", commentContent), nil
	}
	content, err := loadReply(db, *ref.ParentID)
	if err != nil {
		return nil, err
	}
	return notificationSnapshot("reply", *ref.ParentID, "", content), nil
}

func loadArticleCommentReplyContent(db *gorm.DB, replyID uint64) (string, error) {
	var row model.ArticleCommentReply
	if err := db.Select("content").First(&row, replyID).Error; err != nil {
		return "", subjectError(err)
	}
	return row.Content, nil
}

func loadMomentCommentReplyContent(db *gorm.DB, replyID uint64) (string, error) {
	var row model.MomentCommentReply
	if err := db.Select("content").First(&row, replyID).Error; err != nil {
		return "", subjectError(err)
	}
	return row.Content, nil
}

func loadGuestbookReplyContent(db *gorm.DB, replyID uint64) (string, error) {
	var row model.GuestbookReply
	if err := db.Select("content").First(&row, replyID).Error; err != nil {
		return "", subjectError(err)
	}
	return row.Content, nil
}

func articleExcerpt(article model.Article) string {
	if article.ShortContent != nil {
		if excerpt := strings.TrimSpace(*article.ShortContent); excerpt != "" {
			return excerpt
		}
	}
	content := strings.TrimSpace(article.Content)
	if content == "" {
		return ""
	}
	runes := []rune(content)
	if len(runes) <= 200 {
		return content
	}
	return string(runes[:200])
}

func notificationSnapshot(objectType string, id uint64, title, excerpt string) *NotificationSnapshot {
	snapshot := NotificationSnapshot{
		Type:    strings.TrimSpace(objectType),
		ID:      id,
		Title:   truncateNotificationText(strings.TrimSpace(title), notificationSnapshotLimit),
		Excerpt: truncateNotificationText(strings.TrimSpace(excerpt), notificationSnapshotLimit),
	}
	if snapshot.Type == "" && snapshot.ID == 0 && snapshot.Title == "" && snapshot.Excerpt == "" {
		return nil
	}
	return &snapshot
}

func truncateNotificationText(value string, limit int) string {
	if limit <= 0 || value == "" {
		return value
	}
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}
