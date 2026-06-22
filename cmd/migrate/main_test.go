package main

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
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

func TestUpdateMomentMediaURL_DoesNotTouchUpdatedAt(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `moment_media` SET `moment_id`=\\?,`uploader_id`=\\?,`url`=\\? WHERE id = \\? AND `moment_media`.`deleted_at` IS NULL").
		WithArgs(9, 7, "moments/7/9/images/cat.jpg", 5).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = updateMomentMediaURL(gormDB, momentMediaGaragePlan{
		MediaID:           5,
		UpdatedURL:        "moments/7/9/images/cat.jpg",
		UpdatedUploaderID: 7,
		UpdatedMomentID:   9,
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
