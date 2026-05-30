package model

type Media struct {
	Base
	UploaderID uint   `gorm:"index;comment:上传者用户ID" json:"uploader_id"`
	OwnerID    uint   `gorm:"not null;comment:所属实体ID" json:"owner_id"`
	OwnerType  uint8  `gorm:"type:tinyint;not null;comment:所属类型 1=文章 2=说说 3=用户" json:"owner_type"`
	Type       uint8  `gorm:"type:tinyint;default:0;comment:媒体类型 0=图片 1=视频 2=音频" json:"type"`
	FileType   string `gorm:"size:50;comment:文件扩展名（如 jpg、mp4）" json:"file_type"`
	Name       string `gorm:"size:255;comment:原始文件名" json:"name"`
	URL        string `gorm:"size:1000;not null;comment:访问URL" json:"url"`
	Size       uint   `gorm:"comment:文件大小（字节）" json:"size"`
	Status     uint8  `gorm:"type:tinyint;default:1;comment:状态 0=隐藏 1=公开" json:"status"`
	Seq        uint   `gorm:"type:int;default:0;comment:排序" json:"seq"`
	ReadCount  uint   `gorm:"type:int;default:0;comment:查看数" json:"read_count"`
}

func (Media) TableName() string { return "media" }
