package analytics

import (
	"strings"

	xdb "github.com/lionsoul2014/ip2region/binding/golang/xdb"
	"go.uber.org/zap"
)

// GeoResolver 把 IP 解析为国家/地区。
type GeoResolver interface {
	Resolve(ip string) (country, region string)
}

type noopGeo struct{}

func (noopGeo) Resolve(string) (string, string) { return "", "" }

type ip2regionGeo struct {
	searcher *xdb.Searcher
	logger   *zap.Logger
}

// NewGeoResolver 加载 ip2region xdb；路径空或加载失败时降级为 no-op。
func NewGeoResolver(xdbPath string, logger *zap.Logger) GeoResolver {
	if xdbPath == "" {
		logger.Warn("analytics geoip 未配置 xdb 路径，地理解析降级关闭")
		return noopGeo{}
	}
	buf, err := xdb.LoadContentFromFile(xdbPath)
	if err != nil {
		logger.Warn("analytics geoip 加载 xdb 失败，地理解析降级关闭", zap.Error(err))
		return noopGeo{}
	}
	// 新版 xdb 支持 IPv4/IPv6，需先从文件头推断版本再构造 searcher。
	header, err := xdb.LoadHeaderFromBuff(buf)
	if err != nil {
		logger.Warn("analytics geoip 解析 xdb 头失败，地理解析降级关闭", zap.Error(err))
		return noopGeo{}
	}
	version, err := xdb.VersionFromHeader(header)
	if err != nil {
		logger.Warn("analytics geoip 识别 xdb 版本失败，地理解析降级关闭", zap.Error(err))
		return noopGeo{}
	}
	searcher, err := xdb.NewWithBuffer(version, buf)
	if err != nil {
		logger.Warn("analytics geoip 创建 searcher 失败，地理解析降级关闭", zap.Error(err))
		return noopGeo{}
	}
	return &ip2regionGeo{searcher: searcher, logger: logger}
}

// Resolve 返回 (country, region)。ip2region 结果形如 "国家|区域|省份|城市|ISP"。
func (g *ip2regionGeo) Resolve(ip string) (string, string) {
	if ip == "" {
		return "", ""
	}
	region, err := g.searcher.Search(ip)
	if err != nil {
		return "", ""
	}
	parts := strings.Split(region, "|")
	country, province := "", ""
	if len(parts) > 0 {
		country = normalizeGeo(parts[0])
	}
	if len(parts) > 2 {
		province = normalizeGeo(parts[2])
	}
	return country, province
}

func normalizeGeo(s string) string {
	if s == "0" || s == "" {
		return ""
	}
	return s
}
