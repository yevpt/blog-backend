package analytics

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/url"
	"strings"
)

const maxPathLen = 512

// SanitizePath 剥离 query 与 fragment，仅保留 path，并截断长度。
func SanitizePath(rawURL string) string {
	p := rawURL
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	if p == "" {
		p = "/"
	}
	return truncateUTF8Bytes(p, maxPathLen)
}

// truncateUTF8Bytes 按 UTF-8 字节数截断，并确保不会切断多字节字符。
func truncateUTF8Bytes(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	used := 0
	for index, r := range s {
		size := len(string(r))
		if used+size > maxBytes {
			return s[:index]
		}
		used += size
	}
	return s
}

// RefererHost 从完整 referer 取小写 host，失败返回空串。
func RefererHost(rawReferer string) string {
	if rawReferer == "" {
		return ""
	}
	u, err := url.Parse(rawReferer)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// HashIP 对 IP 去除主机段后加 salt 做 sha256，返回 16 位 hex 前缀；不存明文。
func HashIP(ip, salt string) string {
	if ip == "" {
		return ""
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	var masked net.IP
	if v4 := parsed.To4(); v4 != nil {
		masked = v4.Mask(net.CIDRMask(24, 32)) // 去末段
	} else {
		masked = parsed.Mask(net.CIDRMask(64, 128)) // 去后 64 位
	}
	sum := sha256.Sum256([]byte(masked.String() + "|" + salt))
	return hex.EncodeToString(sum[:])[:16]
}
