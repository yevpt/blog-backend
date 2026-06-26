// Package dashboard 定义后台首页汇总出参。
package dashboard

// OverviewSummary 后台首页汇总：内容总量、近期互动、用户统计（非流量块）。
type OverviewSummary struct {
	Content      ContentCounts     `json:"content"`
	Interactions InteractionCounts `json:"interactions"`
	Users        UserCounts        `json:"users"`
}

// ContentCounts 各内容表总量（已排除软删）。
type ContentCounts struct {
	Articles    int64 `json:"articles"`
	Categories  int64 `json:"categories"`
	Tags        int64 `json:"tags"`
	Music       int64 `json:"music"`
	FriendLinks int64 `json:"friend_links"`
}

// InteractionCounts 近 7 天新增互动（评论/留言/动态）。
type InteractionCounts struct {
	NewComments  int64 `json:"new_comments"`
	NewGuestbook int64 `json:"new_guestbook"`
	NewMoments   int64 `json:"new_moments"`
}

// UserCounts 用户统计：总数、今日新增、今日活跃。
type UserCounts struct {
	Total       int64 `json:"total"`
	TodayNew    int64 `json:"today_new"`
	TodayActive int64 `json:"today_active"`
}
