package service

import "strings"

func cleanOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func cleanOptionalUpdateString(value *string) (*string, bool) {
	if value == nil {
		return nil, false
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, true
	}
	return &trimmed, true
}
