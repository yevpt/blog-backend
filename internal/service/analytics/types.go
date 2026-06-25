package analytics

// RawEvent 是 collect 接口接收并交给 Enricher 的原始入参（IP/UA/Origin/UserID 由后端注入）。
type RawEvent struct {
	EventType    string
	VisitorID    string
	SessionID    string
	Path         string
	Title        string
	Referer      string
	UA           string
	IP           string
	Origin       string
	UserID       *uint
	CollectToken string
	Signals      CollectSignals
	// OriginAllowed 标记请求 Origin 是否在允许列表内（由 collect 处理器计算并注入）。
	OriginAllowed bool
	// IsSuspect / SuspectReason 由 DecideSuspect 在 collect 编排阶段写入，再透传给富化。
	IsSuspect     bool
	SuspectReason string
}

// CollectSignals 是 tracker 侧注入的反自动化提示信号。
type CollectSignals struct {
	WebDriver     bool
	NoInteraction bool
}
