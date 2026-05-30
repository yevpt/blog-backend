package model

import "time"

type Music struct {
	Base
	Name        string     `gorm:"size:50;not null;comment:曲名" json:"name"`
	Singer      string     `gorm:"size:50;comment:歌手" json:"singer"`
	Album       string     `gorm:"size:50;comment:专辑" json:"album"`
	SongDate    *time.Time `gorm:"type:date;comment:发行日期" json:"song_date"`
	URL         *string    `gorm:"size:200;comment:音频文件URL" json:"url"`
	CoverImgUrl *string    `gorm:"size:200;comment:封面图URL" json:"cover_img_url"`
	Description *string    `gorm:"size:200;comment:简介" json:"description"`
	Lyric       *string    `gorm:"type:text;comment:歌词" json:"lyric"`
	Duration    uint16     `gorm:"type:smallint unsigned;default:0;comment:时长（秒）" json:"duration"`
	Seq         uint       `gorm:"type:int;default:0;comment:排序" json:"seq"`
}

func (Music) TableName() string { return "music" }

type ArticleMusic struct {
	ID        uint `gorm:"primarykey" json:"id"`
	ArticleID uint `gorm:"not null;index;comment:文章ID" json:"article_id"`
	MusicID   uint `gorm:"not null;comment:音乐ID" json:"music_id"`
}

func (ArticleMusic) TableName() string { return "article_music" }
