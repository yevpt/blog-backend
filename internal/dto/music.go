package dto

type MusicArtistResp struct {
	ID          uint    `json:"id" example:"1"`
	Name        string  `json:"name" example:"문성남"`
	NameZh      *string `json:"name_zh,omitempty" example:"文胜南"`
	DisplayName string  `json:"display_name" example:"문성남 (文胜南)"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
	Description *string `json:"description,omitempty"`
}

type MusicArtistSaveReq struct {
	ID          uint    `json:"id"`
	Name        string  `json:"name" binding:"required,max=100"`
	NameZh      *string `json:"name_zh" binding:"omitempty,max=100"`
	AvatarKey   *string `json:"avatar_key" binding:"omitempty,max=500"`
	Description *string `json:"description" binding:"omitempty,max=500"`
}

type MusicArtistListResp struct {
	List []MusicArtistResp `json:"list"`
}

type MusicAlbumResp struct {
	ID          uint             `json:"id" example:"1"`
	Name        string           `json:"name" example:"Album"`
	Artist      *MusicArtistResp `json:"artist,omitempty"`
	CoverURL    *string          `json:"cover_url,omitempty"`
	ReleaseDate *string          `json:"release_date,omitempty" example:"2024-01-01"`
	Description *string          `json:"description,omitempty"`
}

type MusicAlbumSaveReq struct {
	ID          uint    `json:"id"`
	Name        string  `json:"name" binding:"required,max=150"`
	ArtistID    *uint   `json:"artist_id"`
	CoverKey    *string `json:"cover_key" binding:"omitempty,max=500"`
	ReleaseDate *string `json:"release_date" binding:"omitempty"`
	Description *string `json:"description" binding:"omitempty,max=500"`
}

type MusicAlbumListResp struct {
	List []MusicAlbumResp `json:"list"`
}

type MusicItemResp struct {
	ID                uint              `json:"id" example:"1"`
	Name              string            `json:"name" example:"Song"`
	ArtistDisplayName string            `json:"artist_display_name" example:"Aimer / milet"`
	Artists           []MusicArtistResp `json:"artists"`
	Album             *MusicAlbumResp   `json:"album,omitempty"`
	AlbumTrackNo      uint16            `json:"album_track_no" example:"1"`
	AudioURL          *string           `json:"audio_url,omitempty"`
	CoverURL          *string           `json:"cover_url,omitempty"`
	Duration          uint16            `json:"duration" example:"240"`
	IsPublic          bool              `json:"is_public" example:"true"`
	Seq               uint              `json:"seq" example:"0"`
}

type MusicDetailResp struct {
	MusicItemResp
	Lyric *string `json:"lyric,omitempty"`
}

type MusicListResp struct {
	List []MusicItemResp `json:"list"`
}

type MusicAdminListReq struct {
	Keyword  string `form:"keyword"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size" binding:"omitempty,max=100"`
}

type MusicAdminListResp struct {
	List  []MusicItemResp `json:"list"`
	Total int64           `json:"total"`
}

type MusicSaveReq struct {
	ID                uint    `json:"id"`
	Name              string  `json:"name" binding:"required,max=100"`
	ArtistIDs         []uint  `json:"artist_ids" binding:"required,min=1,max=10"`
	ArtistDisplayName string  `json:"artist_display_name" binding:"omitempty,max=200"`
	AlbumID           *uint   `json:"album_id"`
	AlbumTrackNo      uint16  `json:"album_track_no"`
	AudioKey          string  `json:"audio_key" binding:"required,max=500"`
	AudioSize         uint64  `json:"audio_size"`
	AudioMime         string  `json:"audio_mime" binding:"omitempty,max=100"`
	AudioHash         string  `json:"audio_hash" binding:"omitempty,max=64"`
	Lyric             *string `json:"lyric"`
	Duration          uint16  `json:"duration"`
	IsPublic          bool    `json:"is_public"`
	Seq               uint    `json:"seq"`
}

type MusicUploadResp struct {
	Key  string `json:"key" example:"temp/music/1/audio/hash.mp3"`
	URL  string `json:"url" example:"https://cdn.example.com/music/audio/hash.mp3"`
	Size uint64 `json:"size" example:"123456"`
	Mime string `json:"mime" example:"audio/mpeg"`
	Hash string `json:"hash" example:"abcdef"`
}
