package dto

// SendCodeReq 发送邮箱验证码请求
type SendCodeReq struct {
	Email        string `json:"email" binding:"required,email"`
	CaptchaToken string `json:"captcha_token" binding:"required"`
}

// RegisterReq 注册请求
type RegisterReq struct {
	Email    string  `json:"email" binding:"required,email"`
	Password string  `json:"password" binding:"required,min=8"`
	Code     string  `json:"code" binding:"required,len=6"`
	Nickname *string `json:"nickname"`
}

// LoginReq 登录请求，identifier 可为 username / email / phone
type LoginReq struct {
	Identifier string `json:"identifier" binding:"required"`
	Password   string `json:"password" binding:"required"`
}

// RefreshReq 刷新 token 请求
type RefreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// UserResp 用户信息响应（注册/登录均返回）
type UserResp struct {
	ID       uint     `json:"id"`
	Username string   `json:"username"`
	Email    *string  `json:"email"`
	Nickname *string  `json:"nickname"`
	Roles    []string `json:"roles,omitempty"`
}

// LoginResp 登录成功响应
type LoginResp struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int      `json:"expires_in"` // 单位：秒，固定 7200（2h）
	User         UserResp `json:"user"`
}

// TokenResp 刷新 token 响应
type TokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}
