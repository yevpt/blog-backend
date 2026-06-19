// 文章 Garage 对象迁移脚本。
//
// 默认 dry-run，只打印迁移计划；传入 --apply 后才会复制对象并更新 article 表。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/vpt/blog-backend/internal/migration/garagearticles"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/database"
	"github.com/vpt/blog-backend/pkg/storage"
	"gorm.io/gorm"
)

// articleStore 封装 article 表读取和更新，避免命令入口直接散落 SQL 细节。
type articleStore struct {
	db *gorm.DB
}

// garageCopier 封装 Garage 对象复制能力，复用项目现有 storage.Client 初始化。
type garageCopier struct {
	client *storage.Client
}

// runOptions 是命令行参数解析后的运行选项。
type runOptions struct {
	dryRun bool
}

// summary 记录脚本执行结果，用于结束时输出整体统计。
type summary struct {
	articles int
	planned  int
	copied   int
	skipped  int
	updated  int
	failed   int
}

func main() {
	log.SetFlags(0)

	opts, err := parseFlags()
	if err != nil {
		log.Fatal(err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}

	db, err := database.NewMySQL(&cfg.DB)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}

	store := &articleStore{db: db}
	copier, err := newGarageCopier(cfg)
	if err != nil {
		log.Fatalf("Garage 初始化失败: %v", err)
	}

	failures, result, err := run(context.Background(), store, copier, cfg.Garage.Bucket, opts)
	if err != nil {
		log.Fatalf("迁移执行失败: %v", err)
	}

	printSummary(result, failures, opts)
	if len(failures) > 0 {
		os.Exit(1)
	}
}

func parseFlags() (runOptions, error) {
	apply := flag.Bool("apply", false, "执行真实迁移：复制 Garage 对象并更新 article 表")
	dryRun := flag.Bool("dry-run", false, "只预览迁移计划，不复制对象、不更新数据库")
	flag.Parse()

	if *apply && *dryRun {
		return runOptions{}, errors.New("--apply 和 --dry-run 不能同时使用")
	}

	// 未显式 --apply 时一律 dry-run，避免开发环境误操作。
	return runOptions{dryRun: !*apply || *dryRun}, nil
}

func newGarageCopier(cfg *config.Config) (*garageCopier, error) {
	client, err := storage.NewGarage(&cfg.Garage, &cfg.CDN)
	if err != nil {
		return nil, err
	}
	return &garageCopier{client: client}, nil
}

func run(
	ctx context.Context,
	store *articleStore,
	copier *garageCopier,
	bucket string,
	opts runOptions,
) ([]garagearticles.Failure, summary, error) {
	articles, err := store.listArticles()
	if err != nil {
		return nil, summary{}, err
	}

	result := summary{articles: len(articles)}
	failures := make([]garagearticles.Failure, 0)

	for _, article := range articles {
		plan := garagearticles.BuildArticlePlan(article, garagearticles.PlanOptions{Bucket: bucket})
		if !plan.HasChanges() && len(plan.Failures) == 0 {
			result.skipped++
			continue
		}

		result.planned += len(plan.Assets)
		printPlan(plan, opts)
		failures = append(failures, plan.Failures...)
		if opts.dryRun || len(plan.Failures) > 0 {
			result.failed += len(plan.Failures)
			continue
		}

		copyFailures, copied, skipped := copyAssets(ctx, copier, plan.Assets)
		result.copied += copied
		result.skipped += skipped
		failures = append(failures, copyFailures...)
		if len(copyFailures) > 0 {
			result.failed += len(copyFailures)
			continue
		}

		if err := store.updateArticle(plan); err != nil {
			failure := garagearticles.Failure{
				ArticleID: plan.ArticleID,
				Stage:     "update_db",
				Err:       err,
			}
			failures = append(failures, failure)
			result.failed++
			continue
		}
		result.updated++
	}

	return failures, result, nil
}

func printPlan(plan garagearticles.ArticlePlan, opts runOptions) {
	mode := "APPLY"
	if opts.dryRun {
		mode = "DRY-RUN"
	}
	for _, asset := range plan.Assets {
		log.Printf("%s article_id=%d kind=%s source=%s target=%s",
			mode, asset.ArticleID, asset.Kind, asset.SourceKey, asset.TargetKey)
	}
}

func copyAssets(ctx context.Context, copier *garageCopier, assets []garagearticles.AssetPlan) ([]garagearticles.Failure, int, int) {
	failures := make([]garagearticles.Failure, 0)
	copied := 0
	skipped := 0

	for _, asset := range assets {
		exists, err := copier.objectExists(ctx, asset.TargetKey)
		if err != nil {
			failures = append(failures, copyFailure(asset, "head_target", err))
			continue
		}
		if exists {
			skipped++
			continue
		}

		if err := copier.copyObject(ctx, asset.SourceKey, asset.TargetKey); err != nil {
			failures = append(failures, copyFailure(asset, "copy", err))
			continue
		}
		copied++
	}

	return failures, copied, skipped
}

func copyFailure(asset garagearticles.AssetPlan, stage string, err error) garagearticles.Failure {
	return garagearticles.Failure{
		ArticleID: asset.ArticleID,
		Stage:     stage,
		Source:    asset.SourceKey,
		Target:    asset.TargetKey,
		Err:       err,
	}
}

func (s *articleStore) listArticles() ([]garagearticles.ArticleRow, error) {
	var articles []model.Article
	if err := s.db.Model(&model.Article{}).
		Select("id, cover_img_url, content").
		Order("id ASC").
		Find(&articles).Error; err != nil {
		return nil, err
	}

	rows := make([]garagearticles.ArticleRow, 0, len(articles))
	for _, article := range articles {
		rows = append(rows, garagearticles.ArticleRow{
			ID:          article.ID,
			CoverImgURL: article.CoverImgUrl,
			Content:     article.Content,
		})
	}
	return rows, nil
}

func (s *articleStore) updateArticle(plan garagearticles.ArticlePlan) error {
	updates := map[string]any{}
	if plan.UpdatedCoverImgURL != nil {
		updates["cover_img_url"] = *plan.UpdatedCoverImgURL
	}
	if plan.ContentChanged {
		updates["content"] = plan.UpdatedContent
	}
	if len(updates) == 0 {
		return nil
	}
	return s.db.Model(&model.Article{}).
		Where("id = ?", plan.ArticleID).
		Updates(updates).Error
}

func (c *garageCopier) objectExists(ctx context.Context, key string) (bool, error) {
	return c.client.ObjectExists(ctx, key)
}

func (c *garageCopier) copyObject(ctx context.Context, sourceKey, targetKey string) error {
	_, err := c.client.S3().CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(c.client.Bucket()),
		Key:        aws.String(targetKey),
		CopySource: aws.String(copySource(c.client.Bucket(), sourceKey)),
	})
	return err
}

func copySource(bucket, key string) string {
	parts := strings.Split(strings.TrimLeft(key, "/"), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Trim(bucket, "/") + "/" + strings.Join(parts, "/")
}

func printSummary(result summary, failures []garagearticles.Failure, opts runOptions) {
	mode := "apply"
	if opts.dryRun {
		mode = "dry-run"
	}
	log.Printf("迁移统计 mode=%s articles=%d planned=%d copied=%d skipped=%d updated=%d failed=%d",
		mode, result.articles, result.planned, result.copied, result.skipped, result.updated, len(failures))

	for _, failure := range failures {
		log.Print(formatFailure(failure))
	}
}

func formatFailure(failure garagearticles.Failure) string {
	return fmt.Sprintf(
		"FAILED article_id=%d stage=%s source=%s target=%s error=%s",
		failure.ArticleID,
		failure.Stage,
		failure.Source,
		failure.Target,
		failure.Error(),
	)
}
