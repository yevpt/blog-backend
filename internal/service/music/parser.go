package music

import (
	"regexp"
	"strings"
)

var artistNameWithZhPattern = regexp.MustCompile(`^(.+?)[\s]*[（(](.+?)[）)]$`)

func SplitArtistDisplayName(value string) (string, *string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	matches := artistNameWithZhPattern.FindStringSubmatch(trimmed)
	if len(matches) != 3 {
		return trimmed, nil
	}
	name := strings.TrimSpace(matches[1])
	nameZh := strings.TrimSpace(matches[2])
	if name == "" || nameZh == "" {
		return trimmed, nil
	}
	return name, &nameZh
}

func SplitArtistTokens(value string) []string {
	replacer := strings.NewReplacer(
		" feat. ", "/",
		" feat ", "/",
		" ft. ", "/",
		" ft ", "/",
		"、", "/",
		"，", "/",
		",", "/",
		"&", "/",
	)
	normalized := replacer.Replace(strings.TrimSpace(value))
	parts := strings.Split(normalized, "/")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	return result
}

func ArtistDisplayName(name string, nameZh *string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if nameZh == nil || strings.TrimSpace(*nameZh) == "" {
		return name
	}
	return name + " (" + strings.TrimSpace(*nameZh) + ")"
}
