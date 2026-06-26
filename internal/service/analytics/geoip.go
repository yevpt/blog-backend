package analytics

import (
	"net"
	"strings"

	xdb "github.com/lionsoul2014/ip2region/binding/golang/xdb"
	"go.uber.org/zap"
)

// GeoInfo 是 ip2region 解析出的地理与运营商信息。
type GeoInfo struct {
	Country     string
	Region      string
	City        string
	ISP         string
	CountryCode string
}

// GeoResolver 把 IP 解析为地理与运营商信息。
type GeoResolver interface {
	Resolve(ip string) GeoInfo
}

type noopGeo struct{}

func (noopGeo) Resolve(string) GeoInfo { return GeoInfo{} }

type ip2regionGeo struct {
	v4 *xdb.Searcher
	v6 *xdb.Searcher
}

// NewGeoResolver 加载 ip2region IPv4/IPv6 xdb；路径空或加载失败的版本单独降级。
func NewGeoResolver(v4Path, v6Path string, logger *zap.Logger) GeoResolver {
	if v4Path == "" && v6Path == "" {
		logger.Warn("analytics geoip 未配置 xdb 路径，地理解析降级关闭")
		return noopGeo{}
	}

	v4 := loadGeoSearcher(v4Path, xdb.IPv4, logger)
	v6 := loadGeoSearcher(v6Path, xdb.IPv6, logger)
	if v4 == nil && v6 == nil {
		logger.Warn("analytics geoip xdb 均不可用，地理解析降级关闭")
		return noopGeo{}
	}

	return &ip2regionGeo{v4: v4, v6: v6}
}

func loadGeoSearcher(path string, want *xdb.Version, logger *zap.Logger) *xdb.Searcher {
	if path == "" {
		return nil
	}
	buf, err := xdb.LoadContentFromFile(path)
	if err != nil {
		logger.Warn("analytics geoip 加载 xdb 失败，该版本降级关闭", zap.String("path", path), zap.Error(err))
		return nil
	}
	header, err := xdb.LoadHeaderFromBuff(buf)
	if err != nil {
		logger.Warn("analytics geoip 解析 xdb 头失败，该版本降级关闭", zap.String("path", path), zap.Error(err))
		return nil
	}
	version, err := xdb.VersionFromHeader(header)
	if err != nil {
		logger.Warn("analytics geoip 识别 xdb 版本失败，该版本降级关闭", zap.String("path", path), zap.Error(err))
		return nil
	}
	if version.Id != want.Id {
		logger.Warn("analytics geoip xdb 版本与配置项不匹配，该版本降级关闭",
			zap.String("path", path),
			zap.String("want", want.Name),
			zap.String("got", version.Name),
		)
		return nil
	}
	searcher, err := xdb.NewWithBuffer(version, buf)
	if err != nil {
		logger.Warn("analytics geoip 创建 searcher 失败，该版本降级关闭", zap.String("path", path), zap.Error(err))
		return nil
	}
	return searcher
}

// Resolve 返回 ip2region 3.x 字段：国家、省份/地区、城市、ISP、国家代码。
func (g *ip2regionGeo) Resolve(ip string) GeoInfo {
	if ip == "" {
		return GeoInfo{}
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return GeoInfo{}
	}

	searcher := g.v6
	if parsed.To4() != nil {
		searcher = g.v4
	}
	if searcher == nil {
		return GeoInfo{}
	}

	region, err := searcher.Search(ip)
	if err != nil {
		return GeoInfo{}
	}
	return GeoInfoFromRegion(region)
}

// GeoInfoFromRegion 解析 ip2region 3.x 返回值：国家|省份|城市|ISP|iso-alpha2-code。
func GeoInfoFromRegion(region string) GeoInfo {
	parts := strings.Split(region, "|")
	var info GeoInfo
	if len(parts) > 0 {
		info.Country = normalizeGeo(parts[0])
	}
	if len(parts) > 1 {
		info.Region = normalizeGeo(parts[1])
	}
	if len(parts) > 2 {
		info.City = normalizeGeo(parts[2])
	}
	if len(parts) > 3 {
		info.ISP = normalizeGeo(parts[3])
	}
	if len(parts) > 4 {
		info.CountryCode = normalizeGeo(parts[4])
	}
	return info
}

func normalizeGeo(s string) string {
	if s == "0" || s == "" {
		return ""
	}
	return s
}
