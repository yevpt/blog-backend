package model

type Article struct {
	Base
	Title         string  `gorm:"size:200;not null;comment:标题" json:"title"`
	CoverImgUrl   *string `gorm:"size:500;comment:封面图URL" json:"cover_img_url"`
	ShortContent  *string `gorm:"size:1000;comment:摘要" json:"short_content"`
	Content       string  `gorm:"type:longtext;comment:正文（Markdown）" json:"content"`
	UserID        uint    `gorm:"not null;index;comment:作者ID" json:"user_id"`
	Status        uint8   `gorm:"type:tinyint;default:1;comment:状态 0=隐藏 1=公开 2=加密" json:"status"`
	CommentStatus uint8   `gorm:"type:tinyint;default:1;comment:评论状态 0=关闭 1=开启" json:"comment_status"`
	Password      *string `gorm:"size:50;comment:阅读密码（Status=2 时生效）" json:"-"`
	ReadCount     uint    `gorm:"type:int;default:0;comment:阅读数" json:"read_count"`
}

func (Article) TableName() string { return "article" }

type ArticleRecommend struct {
	Base
	ArticleID uint `gorm:"not null;uniqueIndex;comment:文章ID" json:"article_id"`
	Seq       uint `gorm:"type:int;default:0;comment:推荐顺序" json:"seq"`
}

func (ArticleRecommend) TableName() string { return "article_recommend" }
