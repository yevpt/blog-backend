package bridge

import "errors"

var (
	// ErrTimeout 表示请求 Bridge 超时。
	ErrTimeout = errors.New("OAuth bridge 请求超时")
	// ErrUnauthorized 表示 Bridge HMAC 鉴权失败。
	ErrUnauthorized = errors.New("OAuth bridge 鉴权失败")
	// ErrUnavailable 表示 Bridge 服务不可用。
	ErrUnavailable = errors.New("OAuth bridge 不可用")
	// ErrExchangeFailed 表示 GitHub 授权码换取用户信息失败。
	ErrExchangeFailed = errors.New("GitHub 授权换取用户信息失败")
)
