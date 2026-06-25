package bridge

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	domain "github.com/vpt/blog-backend/internal/oauth"
)

const (
	bridgeTimestampHeader = "X-Bridge-Timestamp"
	bridgeSignatureHeader = "X-Bridge-Signature"
	defaultConnectTimeout = 5 * time.Second
	defaultRequestTimeout = 15 * time.Second
)

// Client 向海外 OAuth Bridge 发起换码请求。
type Client interface {
	ExchangeGitHub(ctx context.Context, req ExchangeRequest) (*domain.Profile, error)
}

type httpClient struct {
	baseURL string
	secret  string
	http    *http.Client
	now     func() time.Time
}

// NewClient 创建 Bridge HTTP 客户端；baseURL 与 secret 均非空时生效。
func NewClient(baseURL, secret string) Client {
	trimmedURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	trimmedSecret := strings.TrimSpace(secret)
	if trimmedURL == "" || trimmedSecret == "" {
		return nil
	}
	return &httpClient{
		baseURL: trimmedURL,
		secret:  trimmedSecret,
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{Timeout: defaultConnectTimeout}).DialContext,
			},
			Timeout: defaultRequestTimeout,
		},
		now: time.Now,
	}
}

func (c *httpClient) ExchangeGitHub(ctx context.Context, req ExchangeRequest) (*domain.Profile, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化 bridge 请求失败: %w", err)
	}

	timestamp := strconv.FormatInt(c.now().Unix(), 10)
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+githubExchangePath,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set(bridgeTimestampHeader, timestamp)
	httpReq.Header.Set(bridgeSignatureHeader, signBridgeRequest(c.secret, timestamp, body))

	resp, err := c.http.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrTimeout
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, ErrTimeout
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: 读取响应失败", ErrUnavailable)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		var profile ProfileResponse
		if err := json.Unmarshal(respBody, &profile); err != nil {
			return nil, fmt.Errorf("%w: 解析响应失败", ErrUnavailable)
		}
		if strings.TrimSpace(profile.ProviderUserID) == "" {
			return nil, fmt.Errorf("%w: 缺少 providerUserId", ErrUnavailable)
		}
		return profile.ToProfile(), nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrUnauthorized
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return nil, ErrExchangeFailed
	default:
		if resp.StatusCode >= http.StatusInternalServerError {
			return nil, ErrUnavailable
		}
		return nil, ErrExchangeFailed
	}
}

func signBridgeRequest(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
