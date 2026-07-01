package model

type Media struct {
	Base
	UploaderID uint   `gorm:"index" json:"uploader_id"`
	MomentID   uint   `gorm:"not null;index" json:"moment_id"`
	Type       uint8  `gorm:"type:tinyint;default:0" json:"type"`
	FileType   string `gorm:"size:50" json:"file_type"`
	Name       string `gorm:"size:255" json:"name"`
	URL        string `gorm:"size:1000;not null" json:"url"`
	Size       uint   `json:"size"`
	Status     uint8  `gorm:"type:tinyint;default:1" json:"status"`
	Seq        uint   `gorm:"type:int;default:0" json:"seq"`
	ReadCount  uint   `gorm:"type:int;default:0" json:"read_count"`
}

func (Media) TableName() string { return "moment_media" }
