package garagearticles

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// AssetKind 表示本次迁移中的对象类型。
type AssetKind string

const (
	// AssetKindCover 表示文章封面图。
	AssetKindCover AssetKind = "cover"
	// AssetKindImage 表示文章正文中的图片。
	AssetKindImage AssetKind = "image"
)

var postImagePattern = regexp.MustCompile(`https?://[^\s"'<>()[\]]*post/images/[^\s"'<>()[\]]*|/?[A-Za-z0-9._~:/?#@!$&*+,;=%-]*post/images/[A-Za-z0-9._~:/?#@!$&*+,;=%-]*`)

// PlanOptions 是构建迁移计划时需要的环境信息。
type PlanOptions struct {
	// Bucket 是 Garage 默认 bucket 名称，用于从 /bucket/object URL 中剥离真实对象 key。
	Bucket string
}

// ArticleRow 是迁移脚本从 article 表读取的最小字段集合。
type ArticleRow struct {
	ID          uint
	CoverImgURL *string
	Content     string
}

// AssetPlan 描述一个 Garage 对象从旧 key 复制到新 key 的操作。
type AssetPlan struct {
	ArticleID uint
	Kind      AssetKind
	Source    string
	SourceKey string
	TargetKey string
}

// Failure 记录迁移过程中可以定位到文章和对象的失败明细。
type Failure struct {
	ArticleID uint
	Stage     string
	Source    string
	Target    string
	Err       error
}

// Error 返回适合脚本输出的错误文案。
func (f Failure) Error() string {
	if f.Err == nil {
		return ""
	}
	return f.Err.Error()
}

// ArticlePlan 描述单篇文章的对象复制计划和数据库更新结果。
type ArticlePlan struct {
	ArticleID          uint
	Assets             []AssetPlan
	UpdatedCoverImgURL *string
	UpdatedContent     string
	ContentChanged     bool
	Failures           []Failure
}

// HasChanges 判断本篇文章是否需要复制对象或更新数据库。
func (p ArticlePlan) HasChanges() bool {
	return len(p.Assets) > 0 || p.UpdatedCoverImgURL != nil || p.ContentChanged
}

// BuildArticlePlan 根据文章当前数据生成 Garage 对象迁移计划。
func BuildArticlePlan(row ArticleRow, opts PlanOptions) ArticlePlan {
	plan := ArticlePlan{
		ArticleID:      row.ID,
		UpdatedContent: row.Content,
	}

	targets := newTargetAllocator(row.ID)
	sourceKeyTargets := map[string]string{}

	if row.CoverImgURL != nil {
		coverPlan, ok, failure := buildCoverPlan(row, opts, targets)
		if failure != nil {
			plan.Failures = append(plan.Failures, *failure)
		}
		if ok {
			plan.Assets = append(plan.Assets, coverPlan)
			plan.UpdatedCoverImgURL = &coverPlan.TargetKey
		}
	}

	content, imagePlans, failures := rewriteContentImages(row, opts, targets, sourceKeyTargets)
	plan.UpdatedContent = content
	plan.ContentChanged = content != row.Content
	plan.Assets = append(plan.Assets, imagePlans...)
	plan.Failures = append(plan.Failures, failures...)

	return plan
}

func buildCoverPlan(row ArticleRow, opts PlanOptions, targets *targetAllocator) (AssetPlan, bool, *Failure) {
	source := strings.TrimSpace(*row.CoverImgURL)
	if source == "" {
		return AssetPlan{}, false, nil
	}

	sourceKey := objectKeyFromValue(source, opts.Bucket)
	if sourceKey == "" || isMigratedCoverKey(row.ID, sourceKey) {
		return AssetPlan{}, false, nil
	}

	targetKey, err := targets.next(AssetKindCover, sourceKey)
	if err != nil {
		return AssetPlan{}, false, &Failure{
			ArticleID: row.ID,
			Stage:     "plan_cover",
			Source:    source,
			Err:       err,
		}
	}

	return AssetPlan{
		ArticleID: row.ID,
		Kind:      AssetKindCover,
		Source:    source,
		SourceKey: sourceKey,
		TargetKey: targetKey,
	}, true, nil
}

func rewriteContentImages(
	row ArticleRow,
	opts PlanOptions,
	targets *targetAllocator,
	sourceKeyTargets map[string]string,
) (string, []AssetPlan, []Failure) {
	matches := postImagePattern.FindAllStringIndex(row.Content, -1)
	if len(matches) == 0 {
		return row.Content, nil, nil
	}

	var builder strings.Builder
	builder.Grow(len(row.Content))
	last := 0
	plans := make([]AssetPlan, 0)
	failures := make([]Failure, 0)

	for _, match := range matches {
		source := row.Content[match[0]:match[1]]
		sourceKey := objectKeyFromValue(source, opts.Bucket)
		targetKey, ok := sourceKeyTargets[sourceKey]
		if !ok {
			var err error
			targetKey, err = targets.next(AssetKindImage, sourceKey)
			if err != nil {
				failures = append(failures, Failure{
					ArticleID: row.ID,
					Stage:     "plan_content",
					Source:    source,
					Err:       err,
				})
				continue
			}
			sourceKeyTargets[sourceKey] = targetKey
			plans = append(plans, AssetPlan{
				ArticleID: row.ID,
				Kind:      AssetKindImage,
				Source:    source,
				SourceKey: sourceKey,
				TargetKey: targetKey,
			})
		}

		builder.WriteString(row.Content[last:match[0]])
		builder.WriteString(targetKey)
		last = match[1]
	}

	builder.WriteString(row.Content[last:])
	return builder.String(), plans, failures
}

func objectKeyFromValue(value, bucket string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	rawPath := value
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		parsed, err := url.Parse(value)
		if err == nil {
			rawPath = parsed.Path
		}
	}

	rawPath = stripURLSuffix(rawPath)
	key := strings.TrimLeft(strings.TrimSpace(rawPath), "/")
	bucket = strings.Trim(bucket, "/")
	if bucket != "" && strings.HasPrefix(key, bucket+"/") {
		key = strings.TrimPrefix(key, bucket+"/")
	}
	return key
}

func stripURLSuffix(value string) string {
	value, _, _ = strings.Cut(value, "?")
	value, _, _ = strings.Cut(value, "#")
	return value
}

func isMigratedCoverKey(articleID uint, key string) bool {
	return strings.HasPrefix(strings.TrimLeft(key, "/"), fmt.Sprintf("articles/%d/cover/", articleID))
}

type targetAllocator struct {
	articleID uint
	used      map[string]string
}

func newTargetAllocator(articleID uint) *targetAllocator {
	return &targetAllocator{
		articleID: articleID,
		used:      map[string]string{},
	}
}

func (a *targetAllocator) next(kind AssetKind, sourceKey string) (string, error) {
	sourceKey = strings.TrimLeft(strings.TrimSpace(sourceKey), "/")
	if sourceKey == "" || strings.HasSuffix(sourceKey, "/") {
		return "", fmt.Errorf("对象 key 缺少文件名: %s", sourceKey)
	}

	fileName := path.Base(sourceKey)
	if fileName == "." || fileName == "/" || fileName == "" {
		return "", fmt.Errorf("对象 key 缺少文件名: %s", sourceKey)
	}

	baseTarget := targetPrefix(a.articleID, kind) + "/" + fileName
	target := baseTarget
	for index := 2; ; index++ {
		owner, exists := a.used[target]
		if !exists || owner == sourceKey {
			a.used[target] = sourceKey
			return target, nil
		}
		target = targetWithIndex(baseTarget, index)
	}
}

func targetPrefix(articleID uint, kind AssetKind) string {
	if kind == AssetKindCover {
		return fmt.Sprintf("articles/%d/cover", articleID)
	}
	return fmt.Sprintf("articles/%d/images", articleID)
}

func targetWithIndex(target string, index int) string {
	ext := path.Ext(target)
	name := strings.TrimSuffix(target, ext)
	return fmt.Sprintf("%s-%d%s", name, index, ext)
}
