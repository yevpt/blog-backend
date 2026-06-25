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
