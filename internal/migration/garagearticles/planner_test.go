package garagearticles_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/migration/garagearticles"
)

func TestBuildArticlePlan_MigratesCoverAndContentImages(t *testing.T) {
	cover := "https://blog-oss.example.com/blog/post/covers/cover.jpg?sign=abc"
	content := `![a](post/images/a.png)
<img src="https://blog-oss.example.com/blog/post/images/b.webp?sign=old">`

	plan := garagearticles.BuildArticlePlan(garagearticles.ArticleRow{
		ID:          12,
		CoverImgURL: &cover,
		Content:     content,
	}, garagearticles.PlanOptions{Bucket: "blog"})

	require.True(t, plan.HasChanges())
	require.NotNil(t, plan.UpdatedCoverImgURL)
	assert.Equal(t, "articles/12/cover/cover.jpg", *plan.UpdatedCoverImgURL)
	assert.Equal(t, "![a](articles/12/images/a.png)\n<img src=\"articles/12/images/b.webp\">", plan.UpdatedContent)

	require.Len(t, plan.Assets, 3)
	assert.Equal(t, garagearticles.AssetKindCover, plan.Assets[0].Kind)
	assert.Equal(t, "post/covers/cover.jpg", plan.Assets[0].SourceKey)
	assert.Equal(t, "articles/12/cover/cover.jpg", plan.Assets[0].TargetKey)
	assert.Equal(t, garagearticles.AssetKindImage, plan.Assets[1].Kind)
	assert.Equal(t, "post/images/a.png", plan.Assets[1].SourceKey)
	assert.Equal(t, "articles/12/images/a.png", plan.Assets[1].TargetKey)
	assert.Equal(t, "post/images/b.webp", plan.Assets[2].SourceKey)
	assert.Equal(t, "articles/12/images/b.webp", plan.Assets[2].TargetKey)
}

func TestBuildArticlePlan_SkipsAlreadyMigratedCoverAndNonPostImages(t *testing.T) {
	cover := "articles/7/cover/cover.jpg"
	content := "![new](articles/7/images/a.png) ![other](images/not-post.png)"

	plan := garagearticles.BuildArticlePlan(garagearticles.ArticleRow{
		ID:          7,
		CoverImgURL: &cover,
		Content:     content,
	}, garagearticles.PlanOptions{Bucket: "blog"})

	assert.False(t, plan.HasChanges())
	assert.Nil(t, plan.UpdatedCoverImgURL)
	assert.Equal(t, content, plan.UpdatedContent)
	assert.Empty(t, plan.Assets)
}

func TestBuildArticlePlan_DeduplicatesRepeatedImagesAndAvoidsFilenameCollision(t *testing.T) {
	content := "![a](post/images/a.png) ![a2](/blog/post/images/a.png) ![nested](post/images/nested/a.png)"

	plan := garagearticles.BuildArticlePlan(garagearticles.ArticleRow{
		ID:      9,
		Content: content,
	}, garagearticles.PlanOptions{Bucket: "blog"})

	assert.Equal(t, "![a](articles/9/images/a.png) ![a2](articles/9/images/a.png) ![nested](articles/9/images/a-2.png)", plan.UpdatedContent)
	require.Len(t, plan.Assets, 2)
	assert.Equal(t, "post/images/a.png", plan.Assets[0].SourceKey)
	assert.Equal(t, "articles/9/images/a.png", plan.Assets[0].TargetKey)
	assert.Equal(t, "post/images/nested/a.png", plan.Assets[1].SourceKey)
	assert.Equal(t, "articles/9/images/a-2.png", plan.Assets[1].TargetKey)
}

func TestBuildArticlePlan_ReportsInvalidCoverWithoutChangingArticle(t *testing.T) {
	cover := "https://blog-oss.example.com/blog/post/covers/"

	plan := garagearticles.BuildArticlePlan(garagearticles.ArticleRow{
		ID:          3,
		CoverImgURL: &cover,
	}, garagearticles.PlanOptions{Bucket: "blog"})

	assert.False(t, plan.HasChanges())
	assert.Nil(t, plan.UpdatedCoverImgURL)
	require.Len(t, plan.Failures, 1)
	assert.Equal(t, uint(3), plan.Failures[0].ArticleID)
	assert.Equal(t, "plan_cover", plan.Failures[0].Stage)
	assert.Equal(t, cover, plan.Failures[0].Source)
}
