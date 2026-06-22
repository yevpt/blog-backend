package model

type Media struct {
	Base
	UploaderID uint   `gorm:"index;comment:上传者用户ID" json:"uploader_id"`
	MomentID   uint   `gorm:"not null;index;comment:碎语ID" json:"moment_id"`
	Type       uint8  `gorm:"type:tinyint;default:0;comment:媒体类型 0=图片 1=视频 2=音频 3=附件" json:"type"`
	FileType   string `gorm:"size:50;comment:文件扩展名（如 jpg、mp4）" json:"file_type"`
	Name       string `gorm:"size:255;comment:原始文件名" json:"name"`
	URL        string `gorm:"size:1000;not null;comment:访问URL" json:"url"`
	Size       uint   `gorm:"comment:文件大小（字节）" json:"size"`
	Status     uint8  `gorm:"type:tinyint;default:1;comment:状态 0=隐藏 1=公开" json:"status"`
	Seq        uint   `gorm:"type:int;default:0;comment:排序" json:"seq"`
	ReadCount  uint   `gorm:"type:int;default:0;comment:查看数" json:"read_count"`
}

func (Media) TableName() string { return "moment_media" }
