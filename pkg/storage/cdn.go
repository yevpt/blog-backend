package storage

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	appconfig "github.com/vpt/blog-backend/pkg/config"
)

// clock 抽象当前时间来源，仅供 CDN 签名内部实现和测试使用。
type clock func() time.Time

// cdnSignerImpl 保存 CDN 签名所需的内部状态。
type cdnSignerImpl struct {
	host               string // CDN 访问域名
	secret             string // CDN 签名密钥
	signQueryName      string // 签名参数名
	timestampQueryName string // 时间戳参数名
	now                clock  // 当前时间来源
}

// newCDNSigner 创建 CDN TypeD 签名器，并填充参数名默认值。
func newCDNSigner(cfg *appconfig.CDNConfig) (*CDNSigner, error) {
	// 先校验配置，确保 host 和 secret 足够生成签名 URL。
	if err := validateCDNConfig(cfg); err != nil {
		return nil, err
	}

	// 构造签名器，缺省参数名按常见 sign/t 兜底。
	impl := &cdnSignerImpl{
		host:               strings.TrimRight(cfg.Host, "/"),
		secret:             cfg.Secret,
		signQueryName:      defaultString(cfg.SignQueryName, "sign"),
		timestampQueryName: defaultString(cfg.TimestampQueryName, "t"),
		now:                time.Now,
	}

	return &CDNSigner{impl: impl}, nil
}

// signPath 使用 TypeD 算法为 CDN 文件路径生成私有读 URL。
func (s *CDNSigner) signPath(filePath string) (string, error) {
	// 空路径没有可签名对象，直接返回空字符串。
	if strings.TrimSpace(filePath) == "" {
		return "", nil
	}

	// 先构造不含签名参数的基础 CDN URL。
	cdnURL, err := s.buildURL(filePath)
	if err != nil {
		return "", err
	}

	// 使用十六进制 Unix 时间戳，保持与参考 TypeD 格式一致。
	timestamp := fmt.Sprintf("%x", s.impl.now().Unix())

	// 签名内容为 secret + 文件路径 + timestamp。
	signature := md5Hex(s.impl.secret + cdnURL.EscapedPath() + timestamp)

	// 将签名和时间戳写入配置指定的 query 参数名。
	query := cdnURL.Query()
	query.Set(s.impl.signQueryName, signature)
	query.Set(s.impl.timestampQueryName, timestamp)
	cdnURL.RawQuery = query.Encode()

	return cdnURL.String(), nil
}

// validateCDNConfig 校验 CDN 签名所需的最小配置。
func validateCDNConfig(cfg *appconfig.CDNConfig) error {
	// 配置对象缺失时直接返回明确错误，避免后续空指针。
	if cfg == nil {
		return errors.New("CDN 配置不能为空")
	}

	// host 和 secret 是生成完整签名 URL 的必要字段。
	if cfg.Host == "" || cfg.Secret == "" {
		return errors.New("CDN host、secret 不能为空")
	}

	return nil
}

// buildURL 根据 CDN host 和文件路径生成待签名 URL。
func (s *CDNSigner) buildURL(filePath string) (*url.URL, error) {
	// 先解析配置中的 CDN host，保留标准 URL 校验能力。
	cdnURL, err := url.Parse(s.impl.host)
	if err != nil {
		return nil, fmt.Errorf("解析 CDN host 失败: %w", err)
	}

	// host 必须包含协议和域名，否则生成的访问 URL 不完整。
	if cdnURL.Scheme == "" || cdnURL.Host == "" {
		return nil, errors.New("解析 CDN host 失败: host 必须包含 scheme 和域名")
	}

	// 覆盖 path 并清空 query/fragment，避免 host 配置中的残留参数参与签名。
	cdnURL.Path = ensureLeadingSlash(filePath)
	cdnURL.RawQuery = ""
	cdnURL.Fragment = ""

	return cdnURL, nil
}

// md5Hex 返回字符串的 MD5 十六进制摘要。
func md5Hex(value string) string {
	// 先计算 MD5 二进制摘要。
	sum := md5.Sum([]byte(value))

	// 再转成 CDN TypeD 签名需要的十六进制字符串。
	return hex.EncodeToString(sum[:])
}
