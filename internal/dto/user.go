package dto

import "time"

// UserDetailResp 当前登录用户详情响应
type UserDetailResp struct {
	ID          uint                 `json:"id"`
	Username    string               `json:"username"`
	Nickname    *string              `json:"nickname,omitempty"`
	Email       *string              `json:"email,omitempty"`
	Phone       *string              `json:"phone,omitempty"`
	Site        *string              `json:"site,omitempty"`
	AvatarUrl   *string              `json:"avatar_url,omitempty"`
	Mark        *string              `json:"mark,omitempty"`
	Status      uint8                `json:"status"`
	LastLoginAt *time.Time           `json:"last_login_at,omitempty"`
	Roles       []string             `json:"roles"`
	Meta        *UserMetaResp        `json:"meta,omitempty"`
	Setting     *UserSettingResp     `json:"setting,omitempty"`
	SocialLinks []UserSocialLinkResp `json:"social_links,omitempty"`
}

// UserMetaResp 用户扩展资料响应
type UserMetaResp struct {
	Name        *string    `json:"name,omitempty"`
	Description *string    `json:"description,omitempty"`
	Gender      *uint8     `json:"gender,omitempty"`
	Birthday    *time.Time `json:"birthday,omitempty"`
	Country     *string    `json:"country,omitempty"`
	Province    *string    `json:"province,omitempty"`
	City        *string    `json:"city,omitempty"`
	Address     *string    `json:"address,omitempty"`
}

// UserSettingResp 用户偏好设置响应
type UserSettingResp struct {
	MailShow     uint8 `json:"mail_show"`
	MailReceive  uint8 `json:"mail_receive"`
	DarkMode     uint8 `json:"dark_mode"`
	ReceiveMail  bool  `json:"receive_mail"`
	ShowName     bool  `json:"show_name"`
	ShowAge      bool  `json:"show_age"`
	ShowPhone    bool  `json:"show_phone"`
	ShowQq       bool  `json:"show_qq"`
	ShowWechat   bool  `json:"show_wechat"`
	ShowZhihu    bool  `json:"show_zhihu"`
	ShowSina     bool  `json:"show_sina"`
	ShowBili     bool  `json:"show_bili"`
	ShowPosition bool  `json:"show_position"`
}

// UserSocialLinkResp 用户社交链接响应
type UserSocialLinkResp struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
}
