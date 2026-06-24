package dto

// MusicItemResp 音乐列表项响应。
type MusicItemResp struct {
	// ID 音乐 ID。
	ID uint `json:"id" example:"1"`
	// Name 音乐名称。
	Name string `json:"name" example:"Song"`
	// Singer 歌手名称。
	Singer string `json:"singer" example:"Singer"`
	// Album 专辑名称。
	Album string `json:"album" example:"Album"`
	// URL 音乐播放地址。
	URL *string `json:"url,omitempty"`
	// CoverImgUrl 音乐封面图地址。
	CoverImgUrl *string `json:"cover_img_url,omitempty"`
	// Duration 音乐时长，单位为秒。
	Duration uint16 `json:"duration" example:"240"`
	// Seq 排序值，越小越靠前。
	Seq uint `json:"seq" example:"0"`
}

// MusicListResp 音乐列表响应。
type MusicListResp struct {
	// List 音乐列表，按 seq ASC、id ASC 排序。
	List []MusicItemResp `json:"list"`
}
