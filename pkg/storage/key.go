package storage

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
)

var (
	ErrExternalObjectURL = errors.New("对象 URL 不属于本站")
	ErrInvalidObjectKey  = errors.New("对象 key 无效")
)

type ObjectKeyParserConfig struct {
	Bucket       string
	AllowedHosts []string
}

type ObjectKeyParser struct {
	bucket       string
	allowedHosts map[string]struct{}
}

func NewObjectKeyParser(cfg ObjectKeyParserConfig) *ObjectKeyParser {
	hosts := make(map[string]struct{}, len(cfg.AllowedHosts))
	for _, host := range cfg.AllowedHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
		host = strings.Trim(host, "/")
		if host != "" {
			hosts[host] = struct{}{}
		}
	}
	return &ObjectKeyParser{bucket: strings.Trim(cfg.Bucket, "/"), allowedHosts: hosts}
}

func (p *ObjectKeyParser) ObjectKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrInvalidObjectKey
	}
	rawPath := value
	if IsAbsoluteURL(value) {
		parsed, err := url.Parse(value)
		if err != nil {
			return "", fmt.Errorf("%w: %s", ErrInvalidObjectKey, value)
		}
		host := strings.ToLower(parsed.Host)
		if _, ok := p.allowedHosts[host]; !ok {
			return "", fmt.Errorf("%w: %s", ErrExternalObjectURL, host)
		}
		rawPath = parsed.Path
	}
	rawPath, _, _ = strings.Cut(rawPath, "?")
	rawPath, _, _ = strings.Cut(rawPath, "#")
	key := strings.TrimLeft(strings.TrimSpace(rawPath), "/")
	if p.bucket != "" && strings.HasPrefix(key, p.bucket+"/") {
		key = strings.TrimPrefix(key, p.bucket+"/")
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == ".." {
			return "", fmt.Errorf("%w: %s", ErrInvalidObjectKey, value)
		}
	}
	if strings.HasSuffix(key, "/") {
		return "", fmt.Errorf("%w: %s", ErrInvalidObjectKey, value)
	}
	key = path.Clean(key)
	if key == "." || key == "/" || strings.HasPrefix(key, "../") || key == ".." {
		return "", fmt.Errorf("%w: %s", ErrInvalidObjectKey, value)
	}
	return key, nil
}
