package model

const (
	// ArticleStatusHidden 表示文章已隐藏，不在公开接口展示。
	ArticleStatusHidden uint8 = 0
	// ArticleStatusPublic 表示文章公开展示。
	ArticleStatusPublic uint8 = 1
	// ArticleStatusEncrypted 表示文章公开可见但正文需密码查看。
	ArticleStatusEncrypted uint8 = 2
	// ArticleStatusDraft 表示文章草稿，仅管理端可见。
	ArticleStatusDraft uint8 = 3

	// ArticleAiModelScopeCover 表示 AI 参与文章配图。
	ArticleAiModelScopeCover = "cover"
	// ArticleAiModelScopeContent 表示 AI 参与文章正文参考。
	ArticleAiModelScopeContent = "content"
)

type Article struct {
	Base
	Title         string  `gorm:"size:200;not null;comment:标题" json:"title"`
	CoverImgUrl   *string `gorm:"size:500;comment:封面图URL" json:"cover_img_url"`
	ShortContent  *string `gorm:"size:1000;comment:摘要" json:"short_content"`
	Content       string  `gorm:"type:longtext;comment:正文（Markdown）" json:"content"`
	UserID        uint    `gorm:"not null;index;comment:作者ID" json:"user_id"`
	Status        uint8   `gorm:"type:tinyint;default:1;comment:状态 0=隐藏 1=公开 2=加密 3=草稿" json:"status"`
	CommentStatus uint8   `gorm:"type:tinyint;default:1;comment:评论状态 0=关闭 1=开启" json:"comment_status"`
	Password            *string `gorm:"size:50;comment:阅读密码（Status=2 时生效）" json:"-"`
	ReadCount           uint    `gorm:"type:int;default:0;comment:阅读数" json:"read_count"`
	CoverAiGenerated    bool    `gorm:"default:false;comment:封面是否AI生成（对外披露）" json:"cover_ai_generated"`
	ContentAiReferenced bool    `gorm:"default:false;comment:正文是否AI参考（对外披露）" json:"content_ai_referenced"`
}

func (Article) TableName() string { return "article" }

type ArticleRecommend struct {
	Base
	ArticleID uint `gorm:"not null;uniqueIndex;comment:文章ID" json:"article_id"`
	Seq       uint `gorm:"type:int;default:0;comment:推荐顺序" json:"seq"`
}

func (ArticleRecommend) TableName() string { return "article_recommend" }

// ArticleAiModel 记录文章披露使用的 AI 模型。
type ArticleAiModel struct {
	ID        uint   `gorm:"primarykey" json:"id"`
	ArticleID uint   `gorm:"not null;index;comment:文章ID" json:"article_id"`
	Scope     string `gorm:"size:20;not null;index;comment:用途 cover=配图 content=正文" json:"scope"`
	ModelName string `gorm:"size:100;not null;comment:模型名称" json:"model_name"`
	Seq       uint   `gorm:"type:int;default:0;comment:展示顺序" json:"seq"`
}

func (ArticleAiModel) TableName() string { return "article_ai_model" }
