package dto

// SendCodeReq 发送邮箱验证码请求
type SendCodeReq struct {
	Email        string `json:"email" binding:"required,email"`
	CaptchaToken string `json:"captcha_token" binding:"required"`
}

// RegisterReq 注册请求（multipart/form-data）
type RegisterReq struct {
	Email    string  `form:"email" binding:"required,email"`
	Password string  `form:"password" binding:"required,min=8"`
	Code     string  `form:"code" binding:"required,len=6"`
	Nickname *string `form:"nickname"`
}

// LoginReq 登录请求，identifier 可为 username / email / phone
type LoginReq struct {
	Identifier string `json:"identifier" binding:"required"`
	Password   string `json:"password" binding:"required"`
}

// AdminLoginReq 管理后台登录请求，仅允许使用用户名和密码。
type AdminLoginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// RefreshReq 刷新 token 请求
type RefreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// PasswordResetCodeReq 发送忘记密码验证码请求
type PasswordResetCodeReq struct {
	Email        string `json:"email" binding:"required,email"`
	CaptchaToken string `json:"captcha_token" binding:"required"`
}

// PasswordResetReq 忘记密码重置请求
type PasswordResetReq struct {
	Email       string `json:"email" binding:"required,email"`
	Code        string `json:"code" binding:"required,len=6"`
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

// UserResp 用户信息响应（注册/登录均返回）
type UserResp struct {
	ID            uint     `json:"id"`
	Username      string   `json:"username"`
	Email         *string  `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	Nickname      *string  `json:"nickname"`
	AvatarUrl     *string  `json:"avatar_url,omitempty"`
	Roles         []string `json:"roles,omitempty"`
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

// OAuthAuthorizeResp 第三方授权地址响应
type OAuthAuthorizeResp struct {
	AuthorizeURL string `json:"authorize_url"`
}

// OAuthBindingResp 第三方账号绑定响应
type OAuthBindingResp struct {
	Source       string `json:"source"`
	SocialUserID uint   `json:"social_user_id"`
}

// OAuthCallbackResp 第三方 callback 处理响应
type OAuthCallbackResp struct {
	Action  string            `json:"action"`
	Login   *LoginResp        `json:"login,omitempty"`
	Binding *OAuthBindingResp `json:"binding,omitempty"`
}
