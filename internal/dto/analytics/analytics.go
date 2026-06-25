package analytics

// CollectRequest 是 /collect 上报载荷（仅可信字段，UA/IP/UserID 由后端注入）。
type CollectRequest struct {
	EventType string `json:"event_type" binding:"required,oneof=page_view heartbeat"`
	Path      string `json:"path"`
	Title     string `json:"title"`
	Referer   string `json:"referer"`
	SessionID string `json:"session_id" binding:"required"`
	Screen    string `json:"screen"`
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
