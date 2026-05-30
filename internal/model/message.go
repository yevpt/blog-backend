package model

type Message struct {
	Base
	Title      *string `gorm:"size:50;comment:通知标题" json:"title"`
	Content    *string `gorm:"size:2000;comment:通知内容" json:"content"`
	Type       string  `gorm:"size:20;not null;comment:消息类型 system/comment/comment_reply/comment_like/comment_reply_like/guestBook" json:"type"`
	TypeID     uint    `gorm:"not null;comment:对应类型的记录ID" json:"type_id"`
	FromUserID uint    `gorm:"not null;index;comment:发送者用户ID" json:"from_user_id"`
	ArticleID  *uint   `gorm:"comment:关联文章ID" json:"article_id"`
	CommentID  *uint   `gorm:"comment:关联评论ID" json:"comment_id"`
}

func (Message) TableName() string { return "message" }

type UserMessage struct {
	Base
	UserID    uint `gorm:"not null;index;comment:消息接收者用户ID" json:"user_id"`
	MessageID uint `gorm:"not null;comment:消息ID" json:"message_id"`
	IsRead    bool `gorm:"type:tinyint;default:0;comment:是否已读" json:"is_read"`
}

func (UserMessage) TableName() string { return "user_message" }
