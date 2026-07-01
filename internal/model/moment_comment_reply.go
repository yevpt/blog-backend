package model

// MomentCommentReply 碎语评论回复。
type MomentCommentReply struct {
	Base
	CommentID     uint   `gorm:"not null;index" json:"comment_id"`
	ToUserID      uint   `gorm:"not null" json:"to_user_id"`
	FromUserID    uint   `gorm:"not null" json:"from_user_id"`
	ParentReplyID uint   `gorm:"default:0" json:"parent_reply_id"`
	Content       string `gorm:"size:2000;not null" json:"content"`
}

func (MomentCommentReply) TableName() string { return "moment_comment_reply" }
