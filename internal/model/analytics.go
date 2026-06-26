package model

import "time"

// AnalyticsEvent 单次 PV 原始事件，短期保留、硬删除（不嵌 Base 软删除）。
type AnalyticsEvent struct {
	ID              uint64 `gorm:"primaryKey"`
	EventType       string `gorm:"type:varchar(16);index"` // page_view
	VisitorID       string `gorm:"type:varchar(64);index"`
	UserID          *uint  `gorm:"index"`
	IsAuthenticated bool   `gorm:"index"`
	SessionID       string `gorm:"type:varchar(64);index"`
	Path            string `gorm:"type:varchar(512);index"`
	Title           string `gorm:"type:varchar(255)"`
	RefererHost     string `gorm:"type:varchar(255)"`
	RefererType     string `gorm:"type:varchar(16)"` // direct/search/social/external/internal
	DeviceType      string `gorm:"type:varchar(16)"` // desktop/mobile/tablet/bot
	Browser         string `gorm:"type:varchar(32)"`
	OS              string `gorm:"type:varchar(32)"`
	Country         string `gorm:"type:varchar(64)"`
	Region          string `gorm:"type:varchar(64)"`
	IPHash          string `gorm:"type:varchar(64)"`
	IsNewVisitor    bool
	IsBot           bool   `gorm:"index"`
	BotReason       string `gorm:"type:varchar(64)"`
	IsSuspect       bool
	SuspectReason   string    `gorm:"type:varchar(64)"`
	CreatedAt       time.Time `gorm:"index"`
}

func (AnalyticsEvent) TableName() string { return "analytics_events" }

// AnalyticsSession 会话聚合，心跳 upsert 更新，硬删除。
type AnalyticsSession struct {
	SessionID       string `gorm:"primaryKey;type:varchar(64)"`
	VisitorID       string `gorm:"type:varchar(64);index"`
	UserID          *uint  `gorm:"index"`
	IsAuthenticated bool
	FirstSeen       time.Time
	LastSeen        time.Time `gorm:"index"`
	PVCount         int
	EntryPath       string `gorm:"type:varchar(512)"`
	ExitPath        string `gorm:"type:varchar(512)"`
	Duration        int    // 秒
	IsBounce        bool
	DeviceType      string `gorm:"type:varchar(16)"`
	Browser         string `gorm:"type:varchar(32)"`
	OS              string `gorm:"type:varchar(32)"`
	Country         string `gorm:"type:varchar(64)"`
	Region          string `gorm:"type:varchar(64)"`
	RefererType     string `gorm:"type:varchar(16)"`
	IsBot           bool
	IsSuspect       bool
}

func (AnalyticsSession) TableName() string { return "analytics_sessions" }

// AnalyticsDaily 每日站点总览（永久），含注册/匿名分档。
type AnalyticsDaily struct {
	Date         string `gorm:"primaryKey;type:varchar(10)"` // YYYY-MM-DD（Asia/Shanghai）
	PV           int
	UV           int
	Sessions     int
	NewVisitors  int
	AvgDuration  int
	BounceRate   float64
	RegisteredPV int
	RegisteredUV int
	AnonymousPV  int
	AnonymousUV  int
}

func (AnalyticsDaily) TableName() string { return "analytics_daily" }

// AnalyticsDailyDim 每日每维度（永久），长表。
type AnalyticsDailyDim struct {
	Date      string `gorm:"primaryKey;type:varchar(10)"`
	Dimension string `gorm:"primaryKey;type:varchar(32)"` // referer_type/device/browser/os/country/user_type
	DimValue  string `gorm:"primaryKey;type:varchar(64)"`
	PV        int
	UV        int
}

func (AnalyticsDailyDim) TableName() string { return "analytics_daily_dim" }

// AnalyticsPageDaily 每日每路径（永久），支撑热门页面排行。
type AnalyticsPageDaily struct {
	Date  string `gorm:"primaryKey;type:varchar(10)"`
	Path  string `gorm:"primaryKey;type:varchar(512)"`
	Title string `gorm:"type:varchar(255)"`
	PV    int
	UV    int
}

func (AnalyticsPageDaily) TableName() string { return "analytics_page_daily" }
