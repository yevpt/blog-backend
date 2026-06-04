package dto

// CaptchaChallengeResp GoCaptcha 滑块挑战响应。
type CaptchaChallengeResp struct {
	ChallengeID string `json:"challenge_id"` // 挑战 ID，校验时原样带回
	MasterImage string `json:"master_image"` // 带 data URI 前缀的主图 JPEG base64
	TileImage   string `json:"tile_image"`   // 带 data URI 前缀的滑块 PNG base64
	TileX       int    `json:"tile_x"`       // 滑块初始 X 坐标
	TileY       int    `json:"tile_y"`       // 滑块初始 Y 坐标
	TileWidth   int    `json:"tile_width"`   // 滑块宽度
	TileHeight  int    `json:"tile_height"`  // 滑块高度
	ImageWidth  int    `json:"image_width"`  // 主图宽度
	ImageHeight int    `json:"image_height"` // 主图高度
}

// CaptchaVerifyReq GoCaptcha 滑块校验请求。
type CaptchaVerifyReq struct {
	ChallengeID string `json:"challenge_id" binding:"required"` // 挑战 ID
	X           int    `json:"x" binding:"required"`            // 用户拖动后的 X 坐标
	Y           int    `json:"y" binding:"required"`            // 用户拖动后的 Y 坐标
}

// CaptchaVerifyResp GoCaptcha 校验通过后的短期通行票据。
type CaptchaVerifyResp struct {
	CaptchaToken string `json:"captcha_token"` // 一次性通行票据，仅用于发送邮箱验证码
}
