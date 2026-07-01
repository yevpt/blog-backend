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
	Title               string  `gorm:"size:200;not null" json:"title"`
	CoverImgUrl         *string `gorm:"size:500" json:"cover_img_url"`
	MobileCoverImgUrl   *string `gorm:"size:500" json:"mobile_cover_img_url"`
	ShortContent        *string `gorm:"size:1000" json:"short_content"`
	Content             string  `gorm:"type:longtext" json:"content"`
	UserID              uint    `gorm:"not null;index" json:"user_id"`
	Status              uint8   `gorm:"type:tinyint;default:1" json:"status"`
	CommentStatus       uint8   `gorm:"type:tinyint;default:1" json:"comment_status"`
	Password            *string `gorm:"size:50" json:"-"`
	ReadCount           uint    `gorm:"type:int;default:0" json:"read_count"`
	CoverAiGenerated    bool    `gorm:"default:false" json:"cover_ai_generated"`
	ContentAiReferenced bool    `gorm:"default:false" json:"content_ai_referenced"`
}

func (Article) TableName() string { return "article" }

type ArticleRecommend struct {
	Base
	ArticleID uint `gorm:"not null;uniqueIndex" json:"article_id"`
	Seq       uint `gorm:"type:int;default:0" json:"seq"`
}

func (ArticleRecommend) TableName() string { return "article_recommend" }

// ArticleAiModel 记录文章披露使用的 AI 模型。
type ArticleAiModel struct {
	ID        uint   `gorm:"primarykey" json:"id"`
	ArticleID uint   `gorm:"not null;index" json:"article_id"`
	Scope     string `gorm:"size:20;not null;index" json:"scope"`
	ModelName string `gorm:"size:100;not null" json:"model_name"`
	Seq       uint   `gorm:"type:int;default:0" json:"seq"`
}

func (ArticleAiModel) TableName() string { return "article_ai_model" }
