package model

// MomentCommentReply 碎语评论回复。
type MomentCommentReply struct {
	Base
	CommentID     uint   `gorm:"not null;index;comment:所属碎语评论ID" json:"comment_id"`
	ToUserID      uint   `gorm:"not null;comment:被回复者用户ID" json:"to_user_id"`
	FromUserID    uint   `gorm:"not null;comment:回复者用户ID" json:"from_user_id"`
	ParentReplyID uint   `gorm:"default:0;comment:上级回复ID，0 表示直接回复评论" json:"parent_reply_id"`
	Content       string `gorm:"size:2000;not null;comment:回复内容" json:"content"`
}

func (MomentCommentReply) TableName() string { return "moment_comment_reply" }
