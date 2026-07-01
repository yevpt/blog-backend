package model

import "time"

type MusicArtist struct {
	Base
	Name        string  `gorm:"size:100;not null;uniqueIndex" json:"name"`
	NameZh      *string `gorm:"size:100" json:"name_zh"`
	AvatarKey   *string `gorm:"size:500" json:"avatar_key"`
	Description *string `gorm:"size:500" json:"description"`
}

func (MusicArtist) TableName() string { return "music_artist" }

type MusicAlbum struct {
	Base
	Name        string     `gorm:"size:150;not null;uniqueIndex:idx_music_album_name_artist" json:"name"`
	ArtistID    *uint      `gorm:"uniqueIndex:idx_music_album_name_artist" json:"artist_id"`
	CoverKey    *string    `gorm:"size:500" json:"cover_key"`
	ReleaseDate *time.Time `gorm:"type:date" json:"release_date"`
	Description *string    `gorm:"size:500" json:"description"`
}

func (MusicAlbum) TableName() string { return "music_album" }

type MusicArtistRelation struct {
	ID       uint   `gorm:"primarykey" json:"id"`
	MusicID  uint   `gorm:"not null;uniqueIndex:idx_music_artist_relation;index" json:"music_id"`
	ArtistID uint   `gorm:"not null;uniqueIndex:idx_music_artist_relation;index" json:"artist_id"`
	Role     string `gorm:"size:20;not null;default:primary;uniqueIndex:idx_music_artist_relation" json:"role"`
	Seq      uint   `gorm:"type:int;default:0" json:"seq"`
}

func (MusicArtistRelation) TableName() string { return "music_artist_relation" }

type Music struct {
	Base
	Name              string     `gorm:"size:50;not null" json:"name"`
	Singer            string     `gorm:"size:50" json:"singer"`
	ArtistDisplayName string     `gorm:"size:200" json:"artist_display_name"`
	Album             string     `gorm:"size:50" json:"album"`
	AlbumID           *uint      `gorm:"index" json:"album_id"`
	AlbumTrackNo      uint16     `gorm:"type:smallint unsigned;default:0" json:"album_track_no"`
	SongDate          *time.Time `gorm:"type:date" json:"song_date"`
	AudioKey          *string    `gorm:"size:500" json:"audio_key"`
	AudioSize         uint64     `gorm:"type:bigint unsigned;default:0" json:"audio_size"`
	AudioMime         string     `gorm:"size:100" json:"audio_mime"`
	AudioHash         string     `gorm:"size:64;index" json:"audio_hash"`
	CoverImgUrl       *string    `gorm:"size:200" json:"cover_img_url"`
	Description       *string    `gorm:"size:200" json:"description"`
	Lyric             *string    `gorm:"type:text" json:"lyric"`
	Duration          uint16     `gorm:"type:smallint unsigned;default:0" json:"duration"`
	Seq               uint       `gorm:"type:int;default:0" json:"seq"`
	IsPublic          bool       `gorm:"default:true;index" json:"is_public"`
}

func (Music) TableName() string { return "music" }

type ArticleMusic struct {
	ID        uint `gorm:"primarykey" json:"id"`
	ArticleID uint `gorm:"not null;index" json:"article_id"`
	MusicID   uint `gorm:"not null" json:"music_id"`
}

func (ArticleMusic) TableName() string { return "article_music" }
