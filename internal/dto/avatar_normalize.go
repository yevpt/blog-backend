package dto

// NormalizeAvatarsReq 归一化老用户头像请求；不传 user_id 时处理全部托管头像用户。
type NormalizeAvatarsReq struct {
	UserID       *uint `json:"user_id" example:"42"`
	ClearInvalid *bool `json:"clear_invalid" example:"false"`
}

// ClearUserAvatarResp 清除用户头像后的结果。
type ClearUserAvatarResp struct {
	UserID uint   `json:"user_id" example:"42"`
	OldKey string `json:"old_key,omitempty" example:"avatar/user/broken.bin"`
}

// NormalizeAvatarItem 单个用户头像归一化结果。
type NormalizeAvatarItem struct {
	UserID  uint   `json:"user_id" example:"42"`
	Status  string `json:"status" example:"updated"`
	OldKey  string `json:"old_key,omitempty" example:"avatar/user/old.png"`
	NewKey  string `json:"new_key,omitempty" example:"avatar/user/abc123.jpg"`
	Message string `json:"message,omitempty" example:"头像对象不存在"`
}

// NormalizeAvatarsResp 批量归一化老用户头像响应。
type NormalizeAvatarsResp struct {
	Scanned        int                   `json:"scanned" example:"20"`
	StorageScanned int                   `json:"storage_scanned,omitempty" example:"8"`
	Updated        int                   `json:"updated" example:"3"`
	Cleared        int                   `json:"cleared" example:"1"`
	Purged         int                   `json:"purged,omitempty" example:"5"`
	Skipped        int                   `json:"skipped" example:"2"`
	OK             int                   `json:"ok" example:"14"`
	Failed         int                   `json:"failed" example:"1"`
	Items          []NormalizeAvatarItem `json:"items"`
}
