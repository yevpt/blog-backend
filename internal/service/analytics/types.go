package analytics

// RawEvent 是 collect 接口接收并交给 Enricher 的原始入参（IP/UA/Origin/UserID 由后端注入）。
type RawEvent struct {
	EventType string
	VisitorID string
	SessionID string
	Path      string
	Title     string
	Referer   string
	UA        string
	IP        string
	Origin    string
	UserID    *uint
	// OriginAllowed 标记请求 Origin 是否在允许列表内（由 collect 处理器计算并注入）。
	OriginAllowed bool
}
