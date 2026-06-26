package analytics

// CollectRequest 是 /collect 上报载荷（仅可信字段，UA/IP/UserID 由后端注入）。
type CollectRequest struct {
	EventType    string         `json:"event_type" binding:"required,oneof=page_view heartbeat"`
	Path         string         `json:"path"`
	Title        string         `json:"title"`
	Referer      string         `json:"referer"`
	SessionID    string         `json:"session_id" binding:"required"`
	Screen       string         `json:"screen"`
	CollectToken string         `json:"collect_token"`
	Signals      CollectSignals `json:"signals"`
}

// CollectSignals 是 tracker 侧提供的反自动化提示信号，仅作为 suspect 参考。
type CollectSignals struct {
	WebDriver     bool `json:"webdriver"`
	NoInteraction bool `json:"no_interaction"`
}

// Overview 后台总览：今日实时 + 在线 + 历史累计，含注册/匿名分档。
type Overview struct {
	TodayPV    int64       `json:"today_pv"`
	TodayUV    int64       `json:"today_uv"`
	Online     int64       `json:"online"`
	TotalPV    int64       `json:"total_pv"`
	TotalUV    int64       `json:"total_uv"`
	Registered SegmentStat `json:"registered"`
	Anonymous  SegmentStat `json:"anonymous"`
}

// SegmentStat 单一分档（注册/匿名）的今日计数。
type SegmentStat struct {
	TodayPV int64 `json:"today_pv"`
	TodayUV int64 `json:"today_uv"`
}

// TrendPoint 趋势图单点：某日某指标的取值。
type TrendPoint struct {
	Date  string `json:"date"`
	Value int    `json:"value"`
}

// PageStat 热门页面排行单项。
type PageStat struct {
	Path  string `json:"path"`
	Title string `json:"title"`
	PV    int    `json:"pv"`
	UV    int    `json:"uv"`
}

// DimensionPoint 维度分布单项：某日某维度取值的 PV/UV。
type DimensionPoint struct {
	Date     string `json:"date"`
	DimValue string `json:"dim_value"`
	PV       int    `json:"pv"`
	UV       int    `json:"uv"`
}

// FriendLinkStat 友链入站来源统计项。
type FriendLinkStat struct {
	FriendLinkID uint    `json:"friend_link_id"`
	FriendName   string  `json:"friend_name"`
	Site         string  `json:"site"`
	SiteHost     string  `json:"site_host"`
	PV           int     `json:"pv"`
	UV           int     `json:"uv"`
	Sessions     int     `json:"sessions"`
	InboundRate  float64 `json:"inbound_rate"`
}

// BackfillResult 回填结果：成功重算的天数与区间。
type BackfillResult struct {
	From string `json:"from"`
	To   string `json:"to"`
	Days int    `json:"days"`
}

// PathSequence 聚合后的访问路径序列，不含 visitor/user/IP。
type PathSequence struct {
	Sequence []string `json:"sequence"`
	Sessions int      `json:"sessions"`
}

// FunnelStep 漏斗步骤的会话留存。
type FunnelStep struct {
	Step           string  `json:"step"`
	Sessions       int     `json:"sessions"`
	ConversionRate float64 `json:"conversion_rate"`
}

// RealtimeStat 后台实时概览：当前在线 + 最近活跃路径（仅聚合）。
type RealtimeStat struct {
	Online      int64          `json:"online"`
	RecentPaths []RealtimePath `json:"recent_paths"`
}

// RealtimePath 最近活跃路径及其活跃访客数。
type RealtimePath struct {
	Path   string `json:"path"`
	Active int    `json:"active"`
}

// PublicSummary 前台公开总览：仅聚合数字。
type PublicSummary struct {
	TodayPV      int64 `json:"today_pv"`
	TodayUV      int64 `json:"today_uv"`
	Online       int64 `json:"online"`
	TotalPV      int64 `json:"total_pv"`
	TotalUV      int64 `json:"total_uv"`
	RegisteredUV int64 `json:"registered_uv"`
	AnonymousUV  int64 `json:"anonymous_uv"`
}

// PublicPageStat 前台热门页面项（仅 path/title/pv）。
type PublicPageStat struct {
	Path  string `json:"path"`
	Title string `json:"title"`
	PV    int    `json:"pv"`
}
