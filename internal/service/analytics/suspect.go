package analytics

// DecideSuspect 综合 Origin、collect token 与反自动化信号判定事件是否可疑。
// 仅标记不拒绝：可疑事件仍入库（带 suspect_reason 便于审计），但不计在线/今日。
// 判定优先级：Origin → token → webdriver；NoInteraction 单独不足以判可疑。
func DecideSuspect(raw RawEvent, tokenOK bool, tokenReason string) (bool, string) {
	if !raw.OriginAllowed {
		return true, "origin_denied"
	}
	if !tokenOK {
		if tokenReason == "" {
			tokenReason = "collect_token_invalid"
		}
		return true, tokenReason
	}
	if raw.Signals.WebDriver {
		return true, "webdriver"
	}
	return false, ""
}
