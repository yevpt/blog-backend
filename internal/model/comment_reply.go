package model

// CommentReply 评论回复，通过 comment_type + comment_id 多态关联到三种评论表
type CommentReply struct {
	Base
	// CommentType 标识所属评论表：1=article_comment 2=moment_comment 3=guestbook
	CommentType   uint8  `gorm:"type:tinyint;not null;index:idx_comment_reply_target,priority:1;comment:评论类型 1=文章 2=说说 3=留言板" json:"comment_type"`
	CommentID     uint   `gorm:"not null;index:idx_comment_reply_target,priority:2;comment:所属评论ID" json:"comment_id"`
	ToUserID      uint   `gorm:"not null;comment:被回复者用户ID" json:"to_user_id"`
	FromUserID    uint   `gorm:"not null;comment:回复者用户ID" json:"from_user_id"`
	ParentReplyID uint   `gorm:"default:0;comment:上级回复ID，0 表示直接回复评论" json:"parent_reply_id"`
	Content       string `gorm:"size:2000;not null;comment:回复内容" json:"content"`
}

func (CommentReply) TableName() string { return "comment_reply" }
