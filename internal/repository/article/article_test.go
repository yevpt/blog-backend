package article_test

import (
	"database/sql/driver"
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
		"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
		"short_content", "content", "user_id", "status", "comment_status",
		"password", "read_count", "cover_ai_generated", "content_ai_referenced",
	}).AddRow(2, now, now, nil, "B", nil, nil, "short", "body", 1, 1, 1, nil, 8, false, false)
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
			"singer", "album", "song_date", "audio_key", "cover_img_url", "description",
			"lyric", "duration", "seq",
		}))
	expectArticleAiModels(mock, 2)
	expectArticleUsers(mock, 1)

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

func TestArticleRepository_ListAdmin_IncludesSoftDeletedArticles(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	now := time.Now()
	deletedAt := now.Add(time.Hour)
	articleRows := sqlmock.NewRows([]string{
		"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
		"short_content", "content", "user_id", "status", "comment_status",
		"password", "read_count", "cover_ai_generated", "content_ai_referenced",
	}).
		AddRow(1, now, now, nil, "Hidden", nil, nil, nil, "body", 7, 0, 1, nil, 1, false, false).
		AddRow(2, now, now, deletedAt, "Deleted", nil, nil, nil, "body", 7, 1, 1, nil, 2, false, false)

	mock.ExpectQuery("SELECT COUNT\\(DISTINCT\\(`article`.`id`\\)\\) FROM `article`$").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery("SELECT article\\.\\* FROM `article` ORDER BY article\\.updated_at DESC,article\\.id DESC LIMIT \\?").
		WithArgs(10).
		WillReturnRows(articleRows)
	mock.ExpectQuery("SELECT target_id, count\\(\\*\\) as count FROM `user_like`").
		WithArgs(uint8(1), uint(1), uint(2)).
		WillReturnRows(sqlmock.NewRows([]string{"target_id", "count"}))
	mock.ExpectQuery("SELECT article_id, count\\(\\*\\) as count FROM `article_comment`").
		WithArgs(uint(1), uint(2)).
		WillReturnRows(sqlmock.NewRows([]string{"article_id", "count"}))
	mock.ExpectQuery("SELECT \\* FROM `article_recommend`").
		WithArgs(uint(1), uint(2)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "article_id", "seq",
		}))
	mock.ExpectQuery("SELECT article_category.article_id, category.\\* FROM `article_category` JOIN category ON category.id = article_category.category_id AND category.deleted_at IS NULL").
		WithArgs(uint(1), uint(2)).
		WillReturnRows(sqlmock.NewRows([]string{
			"article_id", "id", "created_at", "updated_at", "deleted_at", "parent_id",
			"name", "url", "icon", "description", "cover_img_url", "seq",
		}))
	mock.ExpectQuery("SELECT article_tag.article_id, tag.\\* FROM `article_tag` JOIN tag ON tag.id = article_tag.tag_id AND tag.deleted_at IS NULL").
		WithArgs(uint(1), uint(2)).
		WillReturnRows(sqlmock.NewRows([]string{
			"article_id", "id", "created_at", "updated_at", "deleted_at",
			"name", "url", "icon", "description", "cover_img_url", "seq",
		}))
	mock.ExpectQuery("SELECT article_music.article_id, music.\\* FROM `article_music` JOIN music ON music.id = article_music.music_id AND music.deleted_at IS NULL").
		WithArgs(uint(1), uint(2)).
		WillReturnRows(sqlmock.NewRows([]string{
			"article_id", "id", "created_at", "updated_at", "deleted_at", "name",
			"singer", "album", "song_date", "audio_key", "cover_img_url", "description",
			"lyric", "duration", "seq",
		}))
	expectArticleAiModels(mock, 1, 2)
	expectArticleUsers(mock, 7)

	result, err := repo.ListAdmin(article.ArticleListFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Total)
	require.Len(t, result.Articles, 2)
	assert.Equal(t, uint8(0), result.Articles[0].Article.Status)
	assert.True(t, result.Articles[1].Article.DeletedAt.Valid)
	assert.Equal(t, deletedAt, result.Articles[1].Article.DeletedAt.Time)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_ListAdmin_SearchesTitleAndShortContent(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	search := "Go"
	likeSearch := "%Go%"
	mock.ExpectQuery("SELECT COUNT\\(DISTINCT\\(`article`.`id`\\)\\) FROM `article` WHERE \\(article.title LIKE \\? OR article.short_content LIKE \\?\\)$").
		WithArgs(likeSearch, likeSearch).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT article\\.\\* FROM `article` WHERE \\(article.title LIKE \\? OR article.short_content LIKE \\?\\) ORDER BY article\\.updated_at DESC,article\\.id DESC LIMIT \\?").
		WithArgs(likeSearch, likeSearch, 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count", "cover_ai_generated", "content_ai_referenced",
		}))

	result, err := repo.ListAdmin(article.ArticleListFilter{Page: 1, PageSize: 10, Search: &search})
	require.NoError(t, err)
	assert.Equal(t, int64(0), result.Total)
	assert.Empty(t, result.Articles)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_ListAdmin_SortsBySupportedFields(t *testing.T) {
	cases := []struct {
		name         string
		sortBy       string
		sortOrder    string
		countPattern string
		listPattern  string
	}{
		{
			name:         "created_at",
			sortBy:       "created_at",
			sortOrder:    "asc",
			countPattern: "SELECT COUNT\\(DISTINCT\\(`article`.`id`\\)\\) FROM `article`$",
			listPattern:  "SELECT article\\.\\* FROM `article` ORDER BY article\\.created_at ASC,article\\.id DESC LIMIT \\?",
		},
		{
			name:         "updated_at",
			sortBy:       "updated_at",
			sortOrder:    "desc",
			countPattern: "SELECT COUNT\\(DISTINCT\\(`article`.`id`\\)\\) FROM `article`$",
			listPattern:  "SELECT article\\.\\* FROM `article` ORDER BY article\\.updated_at DESC,article\\.id DESC LIMIT \\?",
		},
		{
			name:         "category",
			sortBy:       "category",
			sortOrder:    "asc",
			countPattern: "SELECT COUNT\\(DISTINCT\\(`article`.`id`\\)\\) FROM `article`$",
			listPattern:  "SELECT article\\.\\* FROM `article` LEFT JOIN article_category sort_article_category ON sort_article_category.article_id = article.id LEFT JOIN category sort_category ON sort_category.id = sort_article_category.category_id AND sort_category.deleted_at IS NULL ORDER BY sort_category.name ASC,sort_category.id ASC,article.id DESC LIMIT \\?",
		},
		{
			name:         "status",
			sortBy:       "status",
			sortOrder:    "asc",
			countPattern: "SELECT COUNT\\(DISTINCT\\(`article`.`id`\\)\\) FROM `article`$",
			listPattern:  "SELECT article\\.\\* FROM `article` ORDER BY article\\.status ASC,article\\.id DESC LIMIT \\?",
		},
		{
			name:         "recommended",
			sortBy:       "recommended",
			sortOrder:    "desc",
			countPattern: "SELECT COUNT\\(DISTINCT\\(`article`.`id`\\)\\) FROM `article`$",
			listPattern:  "SELECT article\\.\\* FROM `article` LEFT JOIN article_recommend sort_article_recommend ON sort_article_recommend.article_id = article.id AND sort_article_recommend.deleted_at IS NULL ORDER BY sort_article_recommend.article_id IS NOT NULL DESC,article.id DESC LIMIT \\?",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, sqlDB := newMockDB(t)
			defer sqlDB.Close()
			repo := article.NewArticleRepository(db)

			mock.ExpectQuery(tc.countPattern).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
			mock.ExpectQuery(tc.listPattern).
				WithArgs(10).
				WillReturnRows(sqlmock.NewRows([]string{
					"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
					"short_content", "content", "user_id", "status", "comment_status",
					"password", "read_count", "cover_ai_generated", "content_ai_referenced",
				}))

			result, err := repo.ListAdmin(article.ArticleListFilter{Page: 1, PageSize: 10, SortBy: tc.sortBy, SortOrder: tc.sortOrder})
			require.NoError(t, err)
			assert.Empty(t, result.Articles)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestArticleRepository_FindPublicDetail_NotFound(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(99), uint(1), uint(2), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count", "cover_ai_generated", "content_ai_referenced",
		}))

	detail, err := repo.FindPublicDetail(99, nil)
	require.NoError(t, err)
	assert.Nil(t, detail)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_FindAdminDetail_IncludesSoftDeletedArticle(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	now := time.Now()
	deletedAt := now.Add(time.Hour)
	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(2), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count", "cover_ai_generated", "content_ai_referenced",
		}).AddRow(2, now, now, deletedAt, "Deleted", nil, nil, "summary", "body", 1, 0, 1, nil, 5, false, false))
	expectEmptyArticleAggregateQueries(mock, 2, 1)

	detail, err := repo.FindAdminDetail(2, nil)
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, uint8(0), detail.Article.Status)
	assert.True(t, detail.Article.DeletedAt.Valid)
	assert.Equal(t, deletedAt, detail.Article.DeletedAt.Time)
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
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count", "cover_ai_generated", "content_ai_referenced",
		}).AddRow(11, now, now, nil, "Locked", nil, nil, "summary", "secret", 1, 2, 1, "pwd", 5, false, false))
	expectEmptyArticleAggregateQueries(mock, 11, 1)

	detail, err := repo.FindPublicDetail(11, nil)
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, uint8(2), detail.Article.Status)
	assert.Equal(t, "secret", detail.Article.Content)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_FindPublicDetail_LoadsAuthorUser(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	now := time.Now()
	nickname := "VPT"
	avatar := "avatars/vpt.png"
	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(12), uint(1), uint(2), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count", "cover_ai_generated", "content_ai_referenced",
		}).AddRow(12, now, now, nil, "With Author", nil, nil, "summary", "body", 7, 1, 1, nil, 5, false, false))
	expectEmptyArticleAggregateQueries(mock, 12)
	expectArticleUsers(mock, 7).AddRow(7, now, now, nil, "vpt", "hash", nickname, nil, nil, nil, avatar, nil, 1, nil)

	detail, err := repo.FindPublicDetail(12, nil)
	require.NoError(t, err)
	require.NotNil(t, detail)
	require.NotNil(t, detail.User)
	assert.Equal(t, uint(7), detail.User.ID)
	assert.Equal(t, "vpt", detail.User.Username)
	assert.Equal(t, &nickname, detail.User.Nickname)
	assert.Equal(t, &avatar, detail.User.AvatarUrl)
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
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count", "cover_ai_generated", "content_ai_referenced",
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
	mock.ExpectExec("UPDATE `article` SET `read_count`=read_count \\+ 1 WHERE id = \\? AND status IN \\(\\?,\\?\\) AND `article`.`deleted_at` IS NULL").
		WithArgs(uint(7), uint(1), uint(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count", "cover_ai_generated", "content_ai_referenced",
		}).AddRow(7, now, now, nil, "A", nil, nil, nil, "body", 1, 1, 1, nil, 12, false, false))
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
	mock.ExpectExec("UPDATE `article` SET `read_count`=read_count \\+ 1 WHERE id = \\? AND status IN \\(\\?,\\?\\) AND `article`.`deleted_at` IS NULL").
		WithArgs(uint(8), uint(1), uint(2)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	article, err := repo.IncrementReadCount(8)
	require.NoError(t, err)
	assert.Nil(t, article)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_Save_PreparesArticleAfterIDAllocated(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `article`").
		WillReturnResult(sqlmock.NewResult(45, 1))
	mock.ExpectExec("UPDATE `article` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `article_category` WHERE article_id = \\?").
		WithArgs(uint(45)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO `article_category`").
		WithArgs(uint(45), uint(1)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM `article_tag` WHERE article_id = \\?").
		WithArgs(uint(45)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM `article_music` WHERE article_id = \\?").
		WithArgs(uint(45)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM `article_ai_model` WHERE article_id = \\?").
		WithArgs(uint(45)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE `article_recommend` SET `deleted_at`=\\? WHERE article_id = \\? AND `article_recommend`.`deleted_at` IS NULL").
		WithArgs(sqlmock.AnyArg(), uint(45)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(45), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count", "cover_ai_generated", "content_ai_referenced",
		}).AddRow(45, now, now, nil, "T", nil, nil, nil, "normalized 45", 7, 1, 1, nil, 0, false, false))
	expectEmptyArticleAggregateQueries(mock, 45, 7)

	_, err := repo.Save(article.ArticleSaveData{
		Article:     model.Article{Title: "T", Content: "raw", UserID: 7, Status: 1, CommentStatus: 1},
		CategoryIDs: []uint{1},
		PrepareArticle: func(article model.Article) (model.Article, error) {
			require.Equal(t, uint(45), article.ID)
			article.Content = "normalized 45"
			return article, nil
		},
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
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
	mock.ExpectExec("UPDATE `article` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
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
		WithArgs(uint(7), uint(5), uint(20), uint(7), uint(6), uint(10)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM `article_music` WHERE article_id = \\?").
		WithArgs(uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO `article_music`").
		WithArgs(uint(7), uint(9)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM `article_ai_model` WHERE article_id = \\?").
		WithArgs(uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO `article_recommend`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count", "cover_ai_generated", "content_ai_referenced",
		}).AddRow(7, now, now, nil, "A", cover, nil, shortContent, "body", 1, 1, 1, nil, 0, false, false))
	expectEmptyArticleAggregateQueries(mock, 7, 1)

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
		CategoryIDs: []uint{3},
		Tags: []article.ArticleTagSaveData{
			{TagID: 5, Seq: 20},
			{TagID: 6, Seq: 10},
		},
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

func TestArticleRepository_ToggleLike_CreatesLike(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(1), uint(2), uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count", "cover_ai_generated", "content_ai_referenced",
		}).AddRow(7, now, now, nil, "A", nil, nil, nil, "body", 2, 1, 1, nil, 0, false, false))
	mock.ExpectQuery("SELECT \\* FROM `user_like`").
		WithArgs(uint(7), uint(1), uint8(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "user_id", "target_id", "type",
		}))
	mock.ExpectExec("INSERT INTO `user_like`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(7), uint(1), uint(2), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count", "cover_ai_generated", "content_ai_referenced",
		}).AddRow(7, now, now, nil, "A", nil, nil, nil, "body", 2, 1, 1, nil, 0, false, false))
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
			"singer", "album", "song_date", "audio_key", "cover_img_url", "description",
			"lyric", "duration", "seq",
		}))
	expectArticleAiModels(mock, 7)
	expectArticleUsers(mock, 2)
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

func TestArticleRepository_ToggleLike_HardDeletesExistingLike(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(1), uint(2), uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count", "cover_ai_generated", "content_ai_referenced",
		}).AddRow(7, now, now, nil, "A", nil, nil, nil, "body", 2, 1, 1, nil, 0, false, false))
	mock.ExpectQuery("SELECT \\* FROM `user_like`").
		WithArgs(uint(7), uint(1), uint8(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "user_id", "target_id", "type",
		}).AddRow(12, now, now, nil, 1, 7, article.ArticleLikeType))
	mock.ExpectExec("DELETE FROM `user_like` WHERE `user_like`.`id` = \\?").
		WithArgs(uint(12)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(7), uint(1), uint(2), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count", "cover_ai_generated", "content_ai_referenced",
		}).AddRow(7, now, now, nil, "A", nil, nil, nil, "body", 2, 1, 1, nil, 0, false, false))
	mock.ExpectQuery("SELECT target_id, count\\(\\*\\) as count FROM `user_like`").
		WithArgs(uint8(1), uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"target_id", "count"}))
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
			"singer", "album", "song_date", "audio_key", "cover_img_url", "description",
			"lyric", "duration", "seq",
		}))
	expectArticleAiModels(mock, 7)
	expectArticleUsers(mock, 2)
	mock.ExpectQuery("SELECT `target_id` FROM `user_like`").
		WithArgs(uint8(1), uint(1), uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"target_id"}))

	detail, liked, err := repo.ToggleLike(7, 1)

	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.False(t, liked)
	assert.False(t, detail.IsLiked)
	assert.Equal(t, int64(0), detail.LikeCount)
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
	mock.ExpectExec("DELETE FROM `article_ai_model` WHERE article_id = \\?").
		WithArgs(uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE `article_recommend` SET `deleted_at`=\\? WHERE article_id = \\? AND `article_recommend`.`deleted_at` IS NULL").
		WithArgs(sqlmock.AnyArg(), uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count", "cover_ai_generated", "content_ai_referenced",
		}).AddRow(7, now, now, nil, "A", nil, nil, nil, "body", 1, 1, 1, nil, 0, false, false))
	expectEmptyArticleAggregateQueries(mock, 7, 1)

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

func TestArticleRepository_PermanentDelete_HardDeletesArticleRelations(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	now := time.Now()
	deletedAt := now.Add(time.Minute)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count", "cover_ai_generated", "content_ai_referenced",
		}).AddRow(9, now, now, deletedAt, "A", nil, nil, nil, "body", 7, 1, 1, nil, 0, false, false))
	mock.ExpectQuery("SELECT `id` FROM `article_comment`").
		WithArgs(uint(9)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(21).AddRow(22))
	mock.ExpectQuery("SELECT `id` FROM `article_comment_reply`").
		WithArgs(uint(21), uint(22)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(31))
	mock.ExpectExec("DELETE FROM `user_like` WHERE target_id = \\? AND type = \\?").
		WithArgs(uint(9), uint8(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `user_like` WHERE target_id IN \\(\\?,\\?\\) AND type = \\?").
		WithArgs(uint(21), uint(22), uint8(2)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE FROM `user_like` WHERE target_id IN \\(\\?\\) AND type = \\?").
		WithArgs(uint(31), uint8(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `article_category` WHERE article_id = \\?").
		WithArgs(uint(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `article_tag` WHERE article_id = \\?").
		WithArgs(uint(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `article_music` WHERE article_id = \\?").
		WithArgs(uint(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `article_ai_model` WHERE article_id = \\?").
		WithArgs(uint(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `article_recommend` WHERE article_id = \\?").
		WithArgs(uint(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `article_comment_reply` WHERE comment_id IN \\(\\?,\\?\\)").
		WithArgs(uint(21), uint(22)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `article_comment` WHERE article_id = \\?").
		WithArgs(uint(9)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE FROM `article` WHERE `article`.`id` = \\?").
		WithArgs(uint(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	deleted, err := repo.PermanentDelete(9, 7)

	require.NoError(t, err)
	require.NotNil(t, deleted)
	assert.Equal(t, uint(9), deleted.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func expectEmptyArticleAggregateQueries(mock sqlmock.Sqlmock, articleID uint, userIDs ...uint) {
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
			"singer", "album", "song_date", "audio_key", "cover_img_url", "description",
			"lyric", "duration", "seq",
		}))
	expectArticleAiModels(mock, articleID)
	if len(userIDs) > 0 {
		expectArticleUsers(mock, userIDs...)
	}
}

func expectArticleUsers(mock sqlmock.Sqlmock, userIDs ...uint) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{
		"id", "created_at", "updated_at", "deleted_at", "username", "password",
		"nickname", "email", "phone", "site", "avatar_url", "mark", "status",
		"last_login_at",
	})
	mock.ExpectQuery("SELECT \\* FROM `user`").
		WithArgs(uintArgs(userIDs)...).
		WillReturnRows(rows)
	return rows
}

func uintArgs(ids []uint) []driver.Value {
	args := make([]driver.Value, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	return args
}

func TestArticleRepository_CountExistingMusicIDs_ReturnsMatchCount(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `music` WHERE id IN \\(\\?,\\?\\) AND `music`.`deleted_at` IS NULL").
		WithArgs(uint(1), uint(2)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	count, err := repo.CountExistingMusicIDs([]uint{1, 2})

	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
	assert.NoError(t, mock.ExpectationsWereMet())
}
