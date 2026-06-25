package model

import "time"

type MusicArtist struct {
	Base
	Name        string  `gorm:"size:100;not null;uniqueIndex;comment:歌手名" json:"name"`
	NameZh      *string `gorm:"size:100;comment:中文译名" json:"name_zh"`
	AvatarKey   *string `gorm:"size:500;comment:歌手头像对象 key" json:"avatar_key"`
	Description *string `gorm:"size:500;comment:简介" json:"description"`
}

func (MusicArtist) TableName() string { return "music_artist" }

type MusicAlbum struct {
	Base
	Name        string     `gorm:"size:150;not null;uniqueIndex:idx_music_album_name_artist;comment:专辑名" json:"name"`
	ArtistID    *uint      `gorm:"uniqueIndex:idx_music_album_name_artist;comment:主歌手ID" json:"artist_id"`
	CoverKey    *string    `gorm:"size:500;comment:专辑封面对象 key" json:"cover_key"`
	ReleaseDate *time.Time `gorm:"type:date;comment:发布时间" json:"release_date"`
	Description *string    `gorm:"size:500;comment:简介" json:"description"`
}

func (MusicAlbum) TableName() string { return "music_album" }

type MusicArtistRelation struct {
	ID       uint   `gorm:"primarykey" json:"id"`
	MusicID  uint   `gorm:"not null;uniqueIndex:idx_music_artist_relation;index;comment:音乐ID" json:"music_id"`
	ArtistID uint   `gorm:"not null;uniqueIndex:idx_music_artist_relation;index;comment:歌手ID" json:"artist_id"`
	Role     string `gorm:"size:20;not null;default:primary;uniqueIndex:idx_music_artist_relation;comment:角色" json:"role"`
	Seq      uint   `gorm:"type:int;default:0;comment:展示顺序" json:"seq"`
}

func (MusicArtistRelation) TableName() string { return "music_artist_relation" }

type Music struct {
	Base
	Name              string     `gorm:"size:50;not null;comment:曲名" json:"name"`
	Singer            string     `gorm:"size:50;comment:歌手" json:"singer"`
	ArtistDisplayName string     `gorm:"size:200;comment:歌手展示名" json:"artist_display_name"`
	Album             string     `gorm:"size:50;comment:专辑" json:"album"`
	AlbumID           *uint      `gorm:"index;comment:专辑ID" json:"album_id"`
	AlbumTrackNo      uint16     `gorm:"type:smallint unsigned;default:0;comment:专辑序号" json:"album_track_no"`
	SongDate          *time.Time `gorm:"type:date;comment:发行日期" json:"song_date"`
	URL               *string    `gorm:"size:200;comment:音频文件URL" json:"url"`
	AudioKey          *string    `gorm:"size:500;comment:音频对象 key" json:"audio_key"`
	AudioSize         uint64     `gorm:"type:bigint unsigned;default:0;comment:音频大小" json:"audio_size"`
	AudioMime         string     `gorm:"size:100;comment:音频 MIME" json:"audio_mime"`
	AudioHash         string     `gorm:"size:64;index;comment:音频 hash" json:"audio_hash"`
	CoverImgUrl       *string    `gorm:"size:200;comment:封面图URL" json:"cover_img_url"`
	Description       *string    `gorm:"size:200;comment:简介" json:"description"`
	Lyric             *string    `gorm:"type:text;comment:歌词" json:"lyric"`
	Duration          uint16     `gorm:"type:smallint unsigned;default:0;comment:时长（秒）" json:"duration"`
	Seq               uint       `gorm:"type:int;default:0;comment:排序" json:"seq"`
	IsPublic          bool       `gorm:"default:true;index;comment:是否公开" json:"is_public"`
}

func (Music) TableName() string { return "music" }

type ArticleMusic struct {
	ID        uint `gorm:"primarykey" json:"id"`
	ArticleID uint `gorm:"not null;index;comment:文章ID" json:"article_id"`
	MusicID   uint `gorm:"not null;comment:音乐ID" json:"music_id"`
}

func (ArticleMusic) TableName() string { return "article_music" }
