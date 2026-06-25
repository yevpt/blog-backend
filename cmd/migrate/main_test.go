package main

import (
	"database/sql"
	"database/sql/driver"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/migration/garagearticles"
	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestArticleStatusVarcharToUint8_PreservesDraftStatus(t *testing.T) {
	assert.Equal(t, uint8(3), articleStatusVarcharToUint8(sql.NullString{String: "draft", Valid: true}))
	assert.Equal(t, uint8(3), articleStatusVarcharToUint8(sql.NullString{String: "03", Valid: true}))
}

func TestBuildMusicArtistSeeds_SplitsChineseTranslation(t *testing.T) {
	seeds := buildMusicArtistSeeds("문성남 (文胜南)")

	require.Len(t, seeds, 1)
	assert.Equal(t, "문성남", seeds[0].Name)
	require.NotNil(t, seeds[0].NameZh)
	assert.Equal(t, "文胜南", *seeds[0].NameZh)
}

func TestBuildMusicArtistSeeds_SplitsCollaboration(t *testing.T) {
	seeds := buildMusicArtistSeeds("Aimer / milet feat. 幾田りら")

	require.Len(t, seeds, 3)
	assert.Equal(t, "Aimer", seeds[0].Name)
	assert.Equal(t, "milet", seeds[1].Name)
	assert.Equal(t, "幾田りら", seeds[2].Name)
}

func TestBuildMusicGaragePlan_RewritesAudioAndCover(t *testing.T) {
	albumID := uint(8)
	plan := buildMusicGaragePlan(3, &albumID, "old/song.mp3", "old/cover.jpg")

	assert.Equal(t, "old/song.mp3", plan.SourceAudioKey)
	assert.Contains(t, plan.TargetAudioKey, "music/audio/3/")
	assert.Equal(t, "old/cover.jpg", plan.SourceCoverKey)
	assert.Contains(t, plan.TargetCoverKey, "music/albums/8/cover/")
}

func TestMusicAlbumCoverRaw_PrefersAlbumCoverKey(t *testing.T) {
	albumCover := "music/albums/8/cover/current.jpg"
	legacyCover := "old/cover.jpg"

	got := musicAlbumCoverRaw(&albumCover, &legacyCover)

	assert.Equal(t, "music/albums/8/cover/current.jpg", got)
}

func TestMusicAlbumCoverRaw_FallbackToLegacySongCover(t *testing.T) {
	legacyCover := "old/cover.jpg"

	got := musicAlbumCoverRaw(nil, &legacyCover)

	assert.Equal(t, "old/cover.jpg", got)
}

func TestMusicAlbumGarageCoverCandidates_PrefersAlbumThenLegacy(t *testing.T) {
	albumCover := "album/cover.jpg"
	legacyCover := "legacy/cover.jpg"

	candidates := musicAlbumGarageCoverCandidates(model.MusicAlbum{
		Base:     model.Base{ID: 8},
		CoverKey: &albumCover,
	}, &legacyCover, nil)

	require.Equal(t, []string{"album/cover.jpg", "legacy/cover.jpg"}, candidates)
}

func TestMusicAlbumGarageCoverCandidates_DeduplicatesSameValue(t *testing.T) {
	value := "same/cover.jpg"

	candidates := musicAlbumGarageCoverCandidates(model.MusicAlbum{
		Base:     model.Base{ID: 8},
		CoverKey: &value,
	}, &value, nil)

	require.Equal(t, []string{"same/cover.jpg"}, candidates)
}

func TestMusicAlbumGarageCoverCandidates_IncludesSourceIcons(t *testing.T) {
	srcCover := "legacy/icon.jpg"

	candidates := musicAlbumGarageCoverCandidates(model.MusicAlbum{
		Base: model.Base{ID: 8},
	}, nil, []string{srcCover})

	require.Equal(t, []string{"legacy/icon.jpg"}, candidates)
}

func TestFindLegacyMusicBackgroundImage_ReadsSourceRow(t *testing.T) {
	src, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer src.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT background_img_url FROM music WHERE ID = ?")).
		WithArgs(uint(3)).
		WillReturnRows(sqlmock.NewRows([]string{"background_img_url"}).AddRow("covers/bg.jpg"))

	cover, found, err := findLegacyMusicBackgroundImage(src, 3)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "covers/bg.jpg", cover)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListLegacyMusicCoverByAlbumID_UsesSourceBackgroundImage(t *testing.T) {
	src, srcMock, err := sqlmock.New()
	require.NoError(t, err)
	defer src.Close()

	db, dstMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	dstMock.ExpectQuery(regexp.QuoteMeta("SELECT `id`,`album_id` FROM `music`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "album_id"}).AddRow(3, 8))
	srcMock.ExpectQuery(regexp.QuoteMeta("SELECT background_img_url FROM music WHERE ID = ?")).
		WithArgs(uint(3)).
		WillReturnRows(sqlmock.NewRows([]string{"background_img_url"}).AddRow("covers/bg.jpg"))
	dstMock.ExpectQuery(regexp.QuoteMeta("SELECT `id`,`name` FROM `music_album`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(8, "Album A"))

	byAlbum, err := listLegacyMusicCoverByAlbumID(src, gormDB)
	require.NoError(t, err)
	assert.Equal(t, []string{"covers/bg.jpg"}, byAlbum[8])
	require.NoError(t, srcMock.ExpectationsWereMet())
	require.NoError(t, dstMock.ExpectationsWereMet())
}

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

func TestUserLikeTypeFromLegacy_MapsMomentLikeToType4(t *testing.T) {
	commentTypeMap := map[uint]uint8{
		11: 1,
		12: 2,
		13: 3,
	}
	replyTypeMap := map[uint]uint8{
		21: 3,
		22: 7,
		23: 8,
	}

	assert.Equal(t, uint8(1), userLikeTypeFromLegacy("01", 10, commentTypeMap, replyTypeMap))
	assert.Equal(t, uint8(2), userLikeTypeFromLegacy("02", 11, commentTypeMap, replyTypeMap))
	assert.Equal(t, uint8(6), userLikeTypeFromLegacy("02", 12, commentTypeMap, replyTypeMap))
	assert.Equal(t, uint8(5), userLikeTypeFromLegacy("02", 13, commentTypeMap, replyTypeMap))
	assert.Equal(t, uint8(3), userLikeTypeFromLegacy("03", 21, commentTypeMap, replyTypeMap))
	assert.Equal(t, uint8(7), userLikeTypeFromLegacy("03", 22, commentTypeMap, replyTypeMap))
	assert.Equal(t, uint8(8), userLikeTypeFromLegacy("03", 23, commentTypeMap, replyTypeMap))
	assert.Equal(t, uint8(4), userLikeTypeFromLegacy("04", 30, commentTypeMap, replyTypeMap))
}

func TestMigrateArticleTag_AssignsSeqByArticleInLegacyOrder(t *testing.T) {
	srcDB, srcMock, err := sqlmock.New()
	require.NoError(t, err)
	defer srcDB.Close()

	dstDB, dstMock, err := sqlmock.New()
	require.NoError(t, err)
	defer dstDB.Close()

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      dstDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	srcMock.ExpectQuery(regexp.QuoteMeta("SELECT ID, tag_id, post_id FROM tag_post ORDER BY ID")).
		WillReturnRows(sqlmock.NewRows([]string{"ID", "tag_id", "post_id"}).
			AddRow(10, 5, 7).
			AddRow(11, 6, 7).
			AddRow(12, 9, 8))

	for _, args := range [][]driver.Value{
		{uint(7), uint(5), uint(0), uint(10)},
		{uint(7), uint(6), uint(1), uint(11)},
		{uint(8), uint(9), uint(0), uint(12)},
	} {
		dstMock.ExpectBegin()
		dstMock.ExpectExec("INSERT INTO `article_tag`").
			WithArgs(args...).
			WillReturnResult(sqlmock.NewResult(1, 1))
		dstMock.ExpectCommit()
	}

	err = migrateArticleTag(srcDB, gormDB)

	require.NoError(t, err)
	require.NoError(t, srcMock.ExpectationsWereMet())
	require.NoError(t, dstMock.ExpectationsWereMet())
}

func TestCleanOrphans_CleansNotificationTablesInsteadOfLegacyMessages(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	mock.MatchExpectationsInOrder(false)
	for _, stmt := range []string{
		"DELETE FROM notification_inbox WHERE NOT EXISTS (SELECT 1 FROM notification_event WHERE notification_event.id = notification_inbox.event_id)",
		"DELETE FROM notification_inbox WHERE NOT EXISTS (SELECT 1 FROM user WHERE user.id = notification_inbox.recipient_user_id)",
		"UPDATE notification_event SET actor_user_id = NULL WHERE actor_user_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM user WHERE user.id = notification_event.actor_user_id)",
		"DELETE FROM notification_event WHERE source_type='article' AND NOT EXISTS (SELECT 1 FROM article WHERE article.id = notification_event.source_id)",
		"DELETE FROM notification_event WHERE source_type='moment' AND NOT EXISTS (SELECT 1 FROM moment WHERE moment.id = notification_event.source_id)",
		"DELETE FROM notification_event WHERE source_type='guestbook' AND NOT EXISTS (SELECT 1 FROM guestbook WHERE guestbook.id = notification_event.source_id)",
		"DELETE FROM notification_event WHERE source_type='comment' AND root_type='article' AND NOT EXISTS (SELECT 1 FROM article_comment WHERE article_comment.id = notification_event.source_id)",
		"DELETE FROM notification_event WHERE source_type='comment' AND root_type='article' AND NOT EXISTS (SELECT 1 FROM article WHERE article.id = notification_event.root_id)",
		"DELETE FROM notification_event WHERE source_type='comment' AND root_type='moment' AND NOT EXISTS (SELECT 1 FROM moment_comment WHERE moment_comment.id = notification_event.source_id)",
		"DELETE FROM notification_event WHERE source_type='comment' AND root_type='moment' AND NOT EXISTS (SELECT 1 FROM moment WHERE moment.id = notification_event.root_id)",
		"DELETE FROM notification_event WHERE source_type='comment' AND root_type='guestbook' AND NOT EXISTS (SELECT 1 FROM guestbook WHERE guestbook.id = notification_event.source_id)",
		"DELETE FROM notification_event WHERE source_type='reply' AND root_type='article' AND NOT EXISTS (SELECT 1 FROM article_comment_reply WHERE article_comment_reply.id = notification_event.source_id)",
		"DELETE FROM notification_event WHERE source_type='reply' AND root_type='article' AND NOT EXISTS (SELECT 1 FROM article WHERE article.id = notification_event.root_id)",
		"DELETE FROM notification_event WHERE source_type='reply' AND root_type='moment' AND NOT EXISTS (SELECT 1 FROM moment_comment_reply WHERE moment_comment_reply.id = notification_event.source_id)",
		"DELETE FROM notification_event WHERE source_type='reply' AND root_type='moment' AND NOT EXISTS (SELECT 1 FROM moment WHERE moment.id = notification_event.root_id)",
		"DELETE FROM notification_event WHERE source_type='reply' AND root_type='guestbook' AND NOT EXISTS (SELECT 1 FROM guestbook_reply WHERE guestbook_reply.id = notification_event.source_id)",
		"DELETE FROM notification_event WHERE source_type='reply' AND root_type='guestbook' AND NOT EXISTS (SELECT 1 FROM guestbook WHERE guestbook.id = notification_event.root_id)",
		"DELETE FROM notification_preference WHERE NOT EXISTS (SELECT 1 FROM user WHERE user.id = notification_preference.user_id)",
		"DELETE FROM notification_email_task WHERE NOT EXISTS (SELECT 1 FROM notification_event WHERE notification_event.id = notification_email_task.event_id)",
		"DELETE FROM notification_email_task WHERE NOT EXISTS (SELECT 1 FROM user WHERE user.id = notification_email_task.recipient_user_id)",
		"DELETE FROM notification_email_task WHERE actor_user_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM user WHERE user.id = notification_email_task.actor_user_id)",
		"DELETE FROM notification_email_task WHERE batch_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM notification_email_batch WHERE notification_email_batch.id = notification_email_task.batch_id)",
		"DELETE FROM notification_email_batch WHERE NOT EXISTS (SELECT 1 FROM user WHERE user.id = notification_email_batch.recipient_user_id)",
		"DELETE FROM notification_email_batch_item WHERE NOT EXISTS (SELECT 1 FROM notification_email_batch WHERE notification_email_batch.id = notification_email_batch_item.batch_id)",
		"DELETE FROM notification_email_batch_item WHERE NOT EXISTS (SELECT 1 FROM notification_email_task WHERE notification_email_task.id = notification_email_batch_item.task_id)",
		"DELETE FROM email_send_log WHERE batch_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM notification_email_batch WHERE notification_email_batch.id = email_send_log.batch_id)",
		"DELETE FROM email_quota_usage WHERE scope_type IN ('actor', 'recipient') AND scope_id <> 0 AND NOT EXISTS (SELECT 1 FROM user WHERE user.id = email_quota_usage.scope_id)",
		"DELETE FROM article_recommend WHERE article_id NOT IN (SELECT id FROM article)",
		"DELETE FROM user_like WHERE user_id NOT IN (SELECT id FROM user)",
		"DELETE FROM user_like WHERE type=1 AND target_id NOT IN (SELECT id FROM article)",
		"DELETE FROM user_like WHERE type=2 AND target_id NOT IN (SELECT id FROM article_comment)",
		"DELETE FROM user_like WHERE type=3 AND target_id NOT IN (SELECT id FROM article_comment_reply)",
		"DELETE FROM user_like WHERE type=4 AND target_id NOT IN (SELECT id FROM moment)",
		"DELETE FROM user_like WHERE type=5 AND target_id NOT IN (SELECT id FROM guestbook)",
		"DELETE FROM user_like WHERE type=6 AND target_id NOT IN (SELECT id FROM moment_comment)",
		"DELETE FROM user_like WHERE type=7 AND target_id NOT IN (SELECT id FROM moment_comment_reply)",
		"DELETE FROM user_like WHERE type=8 AND target_id NOT IN (SELECT id FROM guestbook_reply)",
		"DELETE FROM article_tag WHERE article_id NOT IN (SELECT id FROM article)",
		"DELETE FROM article_tag WHERE tag_id NOT IN (SELECT id FROM tag)",
		"DELETE FROM article_category WHERE article_id NOT IN (SELECT id FROM article)",
		"DELETE FROM article_category WHERE category_id NOT IN (SELECT id FROM category)",
		"DELETE FROM article_music WHERE article_id NOT IN (SELECT id FROM article)",
		"DELETE FROM article_music WHERE music_id NOT IN (SELECT id FROM music)",
		"DELETE FROM user_meta WHERE user_id NOT IN (SELECT id FROM user)",
		"DELETE FROM user_setting WHERE user_id NOT IN (SELECT id FROM user)",
		"DELETE FROM user_social_link WHERE user_id NOT IN (SELECT id FROM user)",
		"DELETE FROM user_role WHERE user_id NOT IN (SELECT id FROM user)",
		"DELETE FROM user_role WHERE role_id NOT IN (SELECT id FROM role)",
		"DELETE FROM social_user_auth WHERE social_user_id NOT IN (SELECT id FROM social_user)",
		"DELETE FROM social_user_auth WHERE user_id NOT IN (SELECT id FROM user)",
		"UPDATE category SET parent_id = NULL WHERE parent_id IS NOT NULL AND parent_id NOT IN (SELECT id FROM (SELECT id FROM category) AS valid_category)",
		"DELETE FROM moment_media WHERE moment_id NOT IN (SELECT id FROM moment)",
		"DELETE FROM moment_media WHERE uploader_id NOT IN (SELECT id FROM user)",
		"DELETE FROM article_comment WHERE article_id NOT IN (SELECT id FROM article)",
		"DELETE FROM article_comment WHERE user_id NOT IN (SELECT id FROM user)",
		"DELETE FROM moment_comment WHERE moment_id NOT IN (SELECT id FROM moment)",
		"DELETE FROM moment_comment WHERE user_id NOT IN (SELECT id FROM user)",
		"DELETE FROM guestbook WHERE owner_user_id NOT IN (SELECT id FROM user)",
		"DELETE FROM guestbook WHERE from_user_id NOT IN (SELECT id FROM user)",
		"DELETE FROM article_comment_reply WHERE comment_id NOT IN (SELECT id FROM article_comment)",
		"DELETE FROM article_comment_reply WHERE to_user_id NOT IN (SELECT id FROM user)",
		"DELETE FROM article_comment_reply WHERE from_user_id NOT IN (SELECT id FROM user)",
		"UPDATE article_comment_reply SET parent_reply_id = 0 WHERE parent_reply_id <> 0 AND parent_reply_id NOT IN (SELECT id FROM (SELECT id FROM article_comment_reply) AS valid_reply)",
		"DELETE FROM moment_comment_reply WHERE comment_id NOT IN (SELECT id FROM moment_comment)",
		"DELETE FROM moment_comment_reply WHERE to_user_id NOT IN (SELECT id FROM user)",
		"DELETE FROM moment_comment_reply WHERE from_user_id NOT IN (SELECT id FROM user)",
		"UPDATE moment_comment_reply SET parent_reply_id = 0 WHERE parent_reply_id <> 0 AND parent_reply_id NOT IN (SELECT id FROM (SELECT id FROM moment_comment_reply) AS valid_reply)",
		"DELETE FROM guestbook_reply WHERE comment_id NOT IN (SELECT id FROM guestbook)",
		"DELETE FROM guestbook_reply WHERE to_user_id NOT IN (SELECT id FROM user)",
		"DELETE FROM guestbook_reply WHERE from_user_id NOT IN (SELECT id FROM user)",
		"UPDATE guestbook_reply SET parent_reply_id = 0 WHERE parent_reply_id <> 0 AND parent_reply_id NOT IN (SELECT id FROM (SELECT id FROM guestbook_reply) AS valid_reply)",
		"DELETE FROM notification_email_task WHERE NOT EXISTS (SELECT 1 FROM notification_event WHERE notification_event.id = notification_email_task.event_id)",
	} {
		mock.ExpectExec(regexp.QuoteMeta(stmt)).
			WillReturnResult(sqlmock.NewResult(0, 0))
	}
	for _, table := range []string{
		"article",
		"moment",
		"guestbook",
		"article_comment",
		"moment_comment",
		"article_comment_reply",
		"moment_comment_reply",
		"guestbook_reply",
	} {
		mock.ExpectQuery("SELECT .* FROM `" + table + "`").
			WillReturnRows(sqlmock.NewRows([]string{"id"}))
	}
	mock.ExpectQuery("SELECT .* FROM `notification_event`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "type", "source_type", "source_id", "root_type", "root_id"}))

	err = cleanOrphans(gormDB)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDefragSpecs_UpdateNotificationRefsAndSkipLegacyUserMessage(t *testing.T) {
	specs := defragSpecs()

	for _, spec := range specs {
		assert.NotEqual(t, "user_message", spec.table)
	}

	assertHasDefragRef(t, specs, "article_comment", "notification_event", "source_id", "source_type='comment' AND root_type='article'")
	assertHasDefragRef(t, specs, "moment_comment", "notification_event", "source_id", "source_type='comment' AND root_type='moment'")
	assertHasDefragRef(t, specs, "guestbook", "notification_event", "source_id", "source_type IN ('guestbook','comment') AND root_type='guestbook'")
	assertHasDefragRef(t, specs, "guestbook", "notification_event", "root_id", "source_type IN ('guestbook','comment') AND root_type='guestbook'")
	assertHasDefragRef(t, specs, "guestbook", "notification_event", "root_id", "source_type='reply' AND root_type='guestbook'")
	assertHasDefragRef(t, specs, "article_comment_reply", "article_comment_reply", "parent_reply_id", "parent_reply_id <> 0")
	assertHasDefragRef(t, specs, "article_comment_reply", "notification_event", "source_id", "source_type='reply' AND root_type='article'")
	assertHasDefragRef(t, specs, "moment_comment_reply", "moment_comment_reply", "parent_reply_id", "parent_reply_id <> 0")
	assertHasDefragRef(t, specs, "moment_comment_reply", "notification_event", "source_id", "source_type='reply' AND root_type='moment'")
	assertHasDefragRef(t, specs, "guestbook_reply", "guestbook_reply", "parent_reply_id", "parent_reply_id <> 0")
	assertHasDefragRef(t, specs, "guestbook_reply", "notification_event", "source_id", "source_type='reply' AND root_type='guestbook'")
}

func assertHasDefragRef(t *testing.T, specs []defragSpec, table string, refTable string, col string, where string) {
	t.Helper()
	for _, spec := range specs {
		if spec.table != table {
			continue
		}
		for _, ref := range spec.refs {
			if ref.table == refTable && ref.col == col && ref.where == where {
				return
			}
		}
	}
	t.Fatalf("missing defrag ref table=%s ref=%s.%s where=%s", table, refTable, col, where)
}

func TestBuildArticleGaragePlan_CopiesLegacyAssetsWhenCurrentAlreadyMigrated(t *testing.T) {
	currentCover := "articles/12/cover/cover.jpg"
	legacyCover := "post/covers/cover.jpg"
	current := articleGarageRow{
		Current: garagearticles.ArticleRow{
			ID:          12,
			CoverImgURL: &currentCover,
			Content:     "![a](articles/12/images/a.png)",
		},
		Legacy: &garagearticles.ArticleRow{
			ID:          12,
			CoverImgURL: &legacyCover,
			Content:     "![a](post/images/a.png)",
		},
	}

	plan := buildArticleGaragePlan(current, "blog")

	require.True(t, plan.HasChanges())
	assert.Nil(t, plan.UpdatedCoverImgURL)
	assert.False(t, plan.ContentChanged)
	require.Len(t, plan.Assets, 2)
	assert.Equal(t, "post/covers/cover.jpg", plan.Assets[0].SourceKey)
	assert.Equal(t, "articles/12/cover/cover.jpg", plan.Assets[0].TargetKey)
	assert.Equal(t, "post/images/a.png", plan.Assets[1].SourceKey)
	assert.Equal(t, "articles/12/images/a.png", plan.Assets[1].TargetKey)
}
