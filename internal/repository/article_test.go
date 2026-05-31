package repository_test

import (
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/repository"
)

func TestArticleRepository_ListPublic_SortsAndPaginates(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewArticleRepository(db)

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

	result, err := repo.ListPublic(repository.ArticleListFilter{Page: 2, PageSize: 10, Recommend: &recommend})
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
	repo := repository.NewArticleRepository(db)

	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(uint(99), uint8(1), 1).
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

func TestArticleRepository_ListPublic_FiltersIgnoreDeletedCategoryAndTag(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewArticleRepository(db)

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

	result, err := repo.ListPublic(repository.ArticleListFilter{
		Page:       1,
		PageSize:   10,
		CategoryID: &categoryID,
		TagID:      &tagID,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), result.Total)
	assert.Empty(t, result.Articles)
	assert.NoError(t, mock.ExpectationsWereMet())
}
