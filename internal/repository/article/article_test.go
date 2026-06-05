package article_test

import (
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/model"
	article "github.com/vpt/blog-backend/internal/repository/article"
)

func TestArticleRepository_ListPublic_SortsAndPaginates(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	now := time.Now()
	articleRows := sqlmock.NewRows([]string{
		"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url",
		"short_content", "content", "user_id", "status", "comment_status",
		"password", "read_count",
	}).AddRow(2, now, now, nil, "B", nil, "short", "body", 1, 1, 1, nil, 8)
	recommend := true

	mock.ExpectQuery("SELECT COUNT\\(DISTINCT\\(`article`.`id`\\)\\) FROM `article` JOIN article_recommend ON article_recommend.article_id = article.id AND article_recommend.deleted_at IS NULL WHERE article.status = \\? AND `article`.`deleted_at` IS NULL").
		WithArgs(uint8(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT article\\.\\* FROM `article` JOIN article_recommend ON article_recommend.article_id = article.id AND article_recommend.deleted_at IS NULL WHERE article.status = \\? AND `article`.`deleted_at` IS NULL ORDER BY article_recommend.seq ASC,article.created_at DESC,article.id DESC LIMIT \\? OFFSET \\?").
		WithArgs(uint8(1), 10, 10).
		WillReturnRows(articleRows)
	mock.ExpectQuery("SELECT target_id, count\\(\\*\\) as count FROM `user_like`").
		WithArgs(uint8(1), uint(2)).
		WillReturnRows(sqlmock.NewRows([]string{"target_id", "count"}).AddRow(2, 1))
	mock.ExpectQuery("SELECT article_id, count\\(\\*\\) as count FROM `article_comment`").
		WithArgs(uint(2)).
		WillReturnRows(sqlmock.NewRows([]string{"article_id", "count"}).AddRow(2, 0))
	mock.ExpectQuery("SELECT \\* FROM `article_recommend`").
		WithArgs(uint(2)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "article_id", "seq",
		}))
	mock.ExpectQuery("SELECT article_category.article_id, category.\\* FROM `article_category` JOIN category ON category.id = article_category.category_id AND category.deleted_at IS NULL").
		WithArgs(uint(2)).
		WillReturnRows(sqlmock.NewRows([]string{
			"article_id", "id", "created_at", "updated_at", "deleted_at", "parent_id",
			"name", "url", "icon", "description", "cover_img_url", "seq",
		}))
	mock.ExpectQuery("SELECT article_tag.article_id, tag.\\* FROM `article_tag` JOIN tag ON tag.id = article_tag.tag_id AND tag.deleted_at IS NULL").
		WithArgs(uint(2)).
		WillReturnRows(sqlmock.NewRows([]string{
			"article_id", "id", "created_at", "updated_at", "deleted_at",
			"name", "url", "icon", "description", "cover_img_url", "seq",
		}))
	mock.ExpectQuery("SELECT article_music.article_id, music.\\* FROM `article_music` JOIN music ON music.id = article_music.music_id AND music.deleted_at IS NULL").
		WithArgs(uint(2)).
		WillReturnRows(sqlmock.NewRows([]string{
			"article_id", "id", "created_at", "updated_at", "deleted_at", "name",
			"singer", "album", "song_date", "url", "cover_img_url", "description",
			"lyric", "duration", "seq",
		}))

	result, err := repo.ListPublic(article.ArticleListFilter{Page: 2, PageSize: 10, Recommend: &recommend}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Total)
	assert.Equal(t, 2, result.Page)
	assert.Equal(t, 10, result.PageSize)
	require.Len(t, result.Articles, 1)
	assert.Equal(t, uint(2), result.Articles[0].Article.ID)
	assert.Equal(t, int64(1), result.Articles[0].LikeCount)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_FindPublicDetail_NotFound(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(99), uint(1), uint(2), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count",
		}))

	detail, err := repo.FindPublicDetail(99, nil)
	require.NoError(t, err)
	assert.Nil(t, detail)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_FindPublicDetail_ReturnsEncryptedArticleShell(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	now := time.Now()
	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(11), uint(1), uint(2), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count",
		}).AddRow(11, now, now, nil, "Locked", nil, "summary", "secret", 1, 2, 1, "pwd", 5))
	expectEmptyArticleAggregateQueries(mock, 11)

	detail, err := repo.FindPublicDetail(11, nil)
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, uint8(2), detail.Article.Status)
	assert.Equal(t, "secret", detail.Article.Content)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_ListPublic_FiltersIgnoreDeletedCategoryAndTag(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	categoryID := uint(3)
	tagID := uint(4)

	filterJoinPattern := "FROM `article` JOIN article_category ON article_category.article_id = article.id JOIN category ON category.id = article_category.category_id AND category.deleted_at IS NULL JOIN article_tag ON article_tag.article_id = article.id JOIN tag ON tag.id = article_tag.tag_id AND tag.deleted_at IS NULL"
	mock.ExpectQuery("SELECT COUNT\\(DISTINCT\\(`article`.`id`\\)\\) "+filterJoinPattern).
		WithArgs(uint8(1), categoryID, tagID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT article\\.\\* "+filterJoinPattern+".*ORDER BY article\\.created_at DESC,article\\.id DESC LIMIT \\?").
		WithArgs(uint8(1), categoryID, tagID, 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count",
		}))

	result, err := repo.ListPublic(article.ArticleListFilter{
		Page:       1,
		PageSize:   10,
		CategoryID: &categoryID,
		TagID:      &tagID,
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(0), result.Total)
	assert.Empty(t, result.Articles)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_IncrementReadCount_UsesAtomicUpdate(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `article` SET `read_count`=read_count \\+ 1,`updated_at`=\\? WHERE id = \\? AND status IN \\(\\?,\\?\\) AND `article`.`deleted_at` IS NULL").
		WithArgs(sqlmock.AnyArg(), uint(7), uint(1), uint(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count",
		}).AddRow(7, now, now, nil, "A", nil, nil, "body", 1, 1, 1, nil, 12))
	mock.ExpectCommit()

	article, err := repo.IncrementReadCount(7)
	require.NoError(t, err)
	require.NotNil(t, article)
	assert.Equal(t, uint(12), article.ReadCount)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_IncrementReadCount_HiddenArticleNotFound(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `article` SET `read_count`=read_count \\+ 1,`updated_at`=\\? WHERE id = \\? AND status IN \\(\\?,\\?\\) AND `article`.`deleted_at` IS NULL").
		WithArgs(sqlmock.AnyArg(), uint(8), uint(1), uint(2)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	article, err := repo.IncrementReadCount(8)
	require.NoError(t, err)
	assert.Nil(t, article)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_Save_CreatesArticleAndReplacesRelations(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	now := time.Now()
	shortContent := "摘要"
	cover := "https://example.com/cover.jpg"
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `article`").
		WillReturnResult(sqlmock.NewResult(7, 1))
	mock.ExpectExec("DELETE FROM `article_category` WHERE article_id = \\?").
		WithArgs(uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `article_category`").
		WithArgs(uint(7), uint(3)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM `article_tag` WHERE article_id = \\?").
		WithArgs(uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO `article_tag`").
		WithArgs(uint(7), uint(5)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM `article_music` WHERE article_id = \\?").
		WithArgs(uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO `article_music`").
		WithArgs(uint(7), uint(9)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO `article_recommend`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count",
		}).AddRow(7, now, now, nil, "A", cover, shortContent, "body", 1, 1, 1, nil, 0))
	expectEmptyArticleAggregateQueries(mock, 7)

	result, err := repo.Save(article.ArticleSaveData{
		Article: model.Article{
			Title:         "A",
			CoverImgUrl:   &cover,
			ShortContent:  &shortContent,
			Content:       "body",
			UserID:        1,
			Status:        1,
			CommentStatus: 1,
		},
		CategoryIDs:  []uint{3},
		TagIDs:       []uint{5},
		MusicIDs:     []uint{9},
		Recommend:    true,
		RecommendSeq: 10,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, uint(7), result.Article.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_IsLiked_HiddenArticleNotFound(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `article`").
		WithArgs(uint(8), uint(1), uint(2)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	liked, count, err := repo.IsLiked(8, 1)
	require.Error(t, err)
	assert.False(t, liked)
	assert.Equal(t, int64(0), count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_ToggleLike_CreatesNotificationForOtherAuthor(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(1), uint(2), uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count",
		}).AddRow(7, now, now, nil, "A", nil, nil, "body", 2, 1, 1, nil, 0))
	mock.ExpectQuery("SELECT \\* FROM `user_like`").
		WithArgs(uint(7), uint(1), uint8(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "user_id", "target_id", "type",
		}))
	mock.ExpectExec("INSERT INTO `user_like`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO `message`").
		WillReturnResult(sqlmock.NewResult(12, 1))
	mock.ExpectExec("INSERT INTO `user_message`").
		WillReturnResult(sqlmock.NewResult(13, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(7), uint(1), uint(2), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
		"password", "read_count",
		}).AddRow(7, now, now, nil, "A", nil, nil, "body", 2, 1, 1, nil, 0))
	mock.ExpectQuery("SELECT target_id, count\\(\\*\\) as count FROM `user_like`").
		WithArgs(uint8(1), uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"target_id", "count"}).AddRow(7, 1))
	mock.ExpectQuery("SELECT article_id, count\\(\\*\\) as count FROM `article_comment`").
		WithArgs(uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"article_id", "count"}))
	mock.ExpectQuery("SELECT \\* FROM `article_recommend`").
		WithArgs(uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "article_id", "seq",
		}))
	mock.ExpectQuery("SELECT article_category.article_id, category.\\* FROM `article_category` JOIN category ON category.id = article_category.category_id AND category.deleted_at IS NULL").
		WithArgs(uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{
			"article_id", "id", "created_at", "updated_at", "deleted_at", "parent_id",
			"name", "url", "icon", "description", "cover_img_url", "seq",
		}))
	mock.ExpectQuery("SELECT article_tag.article_id, tag.\\* FROM `article_tag` JOIN tag ON tag.id = article_tag.tag_id AND tag.deleted_at IS NULL").
		WithArgs(uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{
			"article_id", "id", "created_at", "updated_at", "deleted_at",
			"name", "url", "icon", "description", "cover_img_url", "seq",
		}))
	mock.ExpectQuery("SELECT article_music.article_id, music.\\* FROM `article_music` JOIN music ON music.id = article_music.music_id AND music.deleted_at IS NULL").
		WithArgs(uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{
			"article_id", "id", "created_at", "updated_at", "deleted_at", "name",
			"singer", "album", "song_date", "url", "cover_img_url", "description",
			"lyric", "duration", "seq",
		}))
	mock.ExpectQuery("SELECT `target_id` FROM `user_like`").
		WithArgs(uint8(1), uint(1), uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"target_id"}).AddRow(7))

	detail, liked, err := repo.ToggleLike(7, 1)
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.True(t, liked)
	assert.True(t, detail.IsLiked)
	assert.Equal(t, int64(1), detail.LikeCount)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_Save_AllowsRelationOnlyUpdateWhenFieldsUnchanged(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT `id` FROM `article`").
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectExec("UPDATE `article` SET").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM `article_category` WHERE article_id = \\?").
		WithArgs(uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `article_category`").
		WithArgs(uint(7), uint(8)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM `article_tag` WHERE article_id = \\?").
		WithArgs(uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM `article_music` WHERE article_id = \\?").
		WithArgs(uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE `article_recommend` SET `deleted_at`=\\? WHERE article_id = \\? AND `article_recommend`.`deleted_at` IS NULL").
		WithArgs(sqlmock.AnyArg(), uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count",
		}).AddRow(7, now, now, nil, "A", nil, nil, "body", 1, 1, 1, nil, 0))
	expectEmptyArticleAggregateQueries(mock, 7)

	result, err := repo.Save(article.ArticleSaveData{
		Article: model.Article{
			Base:          model.Base{ID: 7},
			Title:         "A",
			Content:       "body",
			UserID:        1,
			Status:        1,
			CommentStatus: 1,
		},
		CategoryIDs: []uint{8},
		Recommend:   false,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, uint(7), result.Article.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func expectEmptyArticleAggregateQueries(mock sqlmock.Sqlmock, articleID uint) {
	mock.ExpectQuery("SELECT target_id, count\\(\\*\\) as count FROM `user_like`").
		WithArgs(uint8(1), articleID).
		WillReturnRows(sqlmock.NewRows([]string{"target_id", "count"}))
	mock.ExpectQuery("SELECT article_id, count\\(\\*\\) as count FROM `article_comment`").
		WithArgs(articleID).
		WillReturnRows(sqlmock.NewRows([]string{"article_id", "count"}))
	mock.ExpectQuery("SELECT \\* FROM `article_recommend`").
		WithArgs(articleID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "article_id", "seq",
		}))
	mock.ExpectQuery("SELECT article_category.article_id, category.\\* FROM `article_category` JOIN category ON category.id = article_category.category_id AND category.deleted_at IS NULL").
		WithArgs(articleID).
		WillReturnRows(sqlmock.NewRows([]string{
			"article_id", "id", "created_at", "updated_at", "deleted_at", "parent_id",
			"name", "url", "icon", "description", "cover_img_url", "seq",
		}))
	mock.ExpectQuery("SELECT article_tag.article_id, tag.\\* FROM `article_tag` JOIN tag ON tag.id = article_tag.tag_id AND tag.deleted_at IS NULL").
		WithArgs(articleID).
		WillReturnRows(sqlmock.NewRows([]string{
			"article_id", "id", "created_at", "updated_at", "deleted_at",
			"name", "url", "icon", "description", "cover_img_url", "seq",
		}))
	mock.ExpectQuery("SELECT article_music.article_id, music.\\* FROM `article_music` JOIN music ON music.id = article_music.music_id AND music.deleted_at IS NULL").
		WithArgs(articleID).
		WillReturnRows(sqlmock.NewRows([]string{
			"article_id", "id", "created_at", "updated_at", "deleted_at", "name",
			"singer", "album", "song_date", "url", "cover_img_url", "description",
			"lyric", "duration", "seq",
		}))
}
