package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMomentMediaGaragePlan_RewritesSayPath(t *testing.T) {
	plan := buildMomentMediaGaragePlan(momentMediaGarageRow{
		ID:       5,
		UserID:   7,
		MomentID: 9,
		URL:      "https://cdn.example.com/blog/say/9/images/cat.jpg?sign=old",
	}, "blog")

	require.True(t, plan.HasChanges())
	require.NoError(t, plan.Err)
	assert.Equal(t, uint(5), plan.MediaID)
	assert.Equal(t, "say/9/images/cat.jpg", plan.SourceKey)
	assert.Equal(t, "moments/7/9/images/cat.jpg", plan.TargetKey)
	assert.Equal(t, "moments/7/9/images/cat.jpg", plan.UpdatedURL)
	assert.Equal(t, uint(7), plan.UpdatedUploaderID)
	assert.Equal(t, uint(9), plan.UpdatedMomentID)
}

func TestBuildMomentMediaGaragePlan_UsesMomentIDFromOwnerInsteadOfURL(t *testing.T) {
	plan := buildMomentMediaGaragePlan(momentMediaGarageRow{
		ID:                5,
		CurrentUploaderID: 0,
		CurrentMomentID:   4,
		UserID:            2,
		MomentID:          4,
		URL:               "say/2/87e2b8b7153de5689a6bb7406618fa55.jpeg",
	}, "blog")

	require.True(t, plan.HasChanges())
	require.NoError(t, plan.Err)
	assert.Equal(t, "say/2/87e2b8b7153de5689a6bb7406618fa55.jpeg", plan.SourceKey)
	assert.Equal(t, "moments/2/4/87e2b8b7153de5689a6bb7406618fa55.jpeg", plan.TargetKey)
	assert.Equal(t, uint(2), plan.UpdatedUploaderID)
	assert.Equal(t, uint(4), plan.UpdatedMomentID)
}

func TestBuildMomentMediaGaragePlan_SkipsAlreadyMigratedPath(t *testing.T) {
	plan := buildMomentMediaGaragePlan(momentMediaGarageRow{
		ID:       6,
		UserID:   7,
		MomentID: 9,
		URL:      "moments/7/9/images/cat.jpg",
	}, "blog")

	assert.False(t, plan.HasChanges())
	assert.NoError(t, plan.Err)
}

func TestBuildMomentMediaGaragePlan_SkipsNonSayPath(t *testing.T) {
	plan := buildMomentMediaGaragePlan(momentMediaGarageRow{
		ID:       7,
		UserID:   7,
		MomentID: 9,
		URL:      "avatar/cat.jpg",
	}, "blog")

	assert.False(t, plan.HasChanges())
	assert.NoError(t, plan.Err)
}

func TestBuildMomentMediaGaragePlan_ReportsMissingFileName(t *testing.T) {
	plan := buildMomentMediaGaragePlan(momentMediaGarageRow{
		ID:       8,
		UserID:   7,
		MomentID: 9,
		URL:      "say/9/",
	}, "blog")

	assert.False(t, plan.HasChanges())
	require.Error(t, plan.Err)
	assert.Contains(t, plan.Err.Error(), "对象 key 缺少文件名")
}
