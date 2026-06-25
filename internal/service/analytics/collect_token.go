package analytics

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const collectTokenAudience = "analytics_collect"

type collectTokenPayload struct {
	Aud string `json:"aud"`
	Exp int64  `json:"exp"`
}

// CollectTokenVerifier 校验 web SSR 签发的短期 HMAC collect token；空 secret 时开发放行。
type CollectTokenVerifier interface {
	Verify(token string) (ok bool, reason string)
}

type collectTokenVerifier struct {
	secret string
	ttl    time.Duration
	now    func() time.Time
}

// NewCollectTokenVerifier 创建 collect token 校验器。
func NewCollectTokenVerifier(secret string, ttl time.Duration, now func() time.Time) CollectTokenVerifier {
	if now == nil {
		now = time.Now
	}
	return &collectTokenVerifier{secret: secret, ttl: ttl, now: now}
}

func (v *collectTokenVerifier) Verify(token string) (bool, string) {
	if v.secret == "" {
		return true, ""
	}
	if token == "" {
		return false, "collect_token_missing"
	}

	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false, "collect_token_invalid"
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false, "collect_token_invalid"
	}
	expected := signCollectTokenPayload(v.secret, parts[0])
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return false, "collect_token_invalid"
	}

	var payload collectTokenPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil || payload.Aud != collectTokenAudience {
		return false, "collect_token_invalid"
	}
	if v.now().Unix() > payload.Exp {
		return false, "collect_token_expired"
	}
	if v.ttl > 0 && payload.Exp > v.now().Add(v.ttl).Unix() {
		return false, "collect_token_invalid"
	}

	return true, ""
}

func signCollectTokenPayload(secret, encodedPayload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(encodedPayload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// SignCollectTokenForTest 签发测试用 collect token，供前端按同算法对齐。
func SignCollectTokenForTest(secret string, exp time.Time) (string, error) {
	raw, err := json.Marshal(collectTokenPayload{Aud: collectTokenAudience, Exp: exp.Unix()})
	if err != nil {
		return "", fmt.Errorf("marshal collect token: %w", err)
	}

	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + "." + signCollectTokenPayload(secret, payload), nil
}
