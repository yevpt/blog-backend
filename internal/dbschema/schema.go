package dbschema

import (
	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// AutoMigrate 按当前代码模型创建或补齐数据库结构。
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.Role{},
		&model.User{},
		&model.UserRole{},
		&model.UserLike{},
		&model.UserMeta{},
		&model.UserSetting{},
		&model.UserSocialLink{},
		&model.SocialUser{},
		&model.SocialUserAuth{},
		&model.Article{},
		&model.ArticleRecommend{},
		&model.ArticleCategory{},
		&model.ArticleTag{},
		&model.ArticleMusic{},
		&model.Category{},
		&model.Tag{},
		&model.MusicArtist{},
		&model.MusicAlbum{},
		&model.MusicArtistRelation{},
		&model.Music{},
		&model.Moment{},
		&model.FriendLink{},
		&model.Media{},
		&model.ArticleComment{},
		&model.MomentComment{},
		&model.Guestbook{},
		&model.ArticleCommentReply{},
		&model.MomentCommentReply{},
		&model.GuestbookReply{},
		&model.NotificationEvent{},
		&model.NotificationInbox{},
		&model.NotificationPreference{},
		&model.NotificationEmailTask{},
		&model.NotificationEmailBatch{},
		&model.NotificationEmailBatchItem{},
		&model.EmailQuotaPolicy{},
		&model.EmailRoleQuotaPolicy{},
		&model.EmailQuotaUsage{},
		&model.EmailSendLog{},
	)
}
