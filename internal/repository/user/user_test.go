package user_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/vpt/blog-backend/internal/model"
	user "github.com/vpt/blog-backend/internal/repository/user"
)

func newMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	return gormDB, mock, sqlDB
}

func TestUserRepository_FindByIdentifier_Found(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	email := "test@example.com"
	rows := sqlmock.NewRows([]string{
		"id", "created_at", "updated_at", "deleted_at",
		"username", "password", "password_set", "nickname", "email", "phone",
		"site", "avatar_url", "mark", "status", "last_login_at",
	}).AddRow(1, nil, nil, nil, email, "hashed", true, nil, email, nil, nil, nil, nil, 1, nil)

	mock.ExpectQuery(`SELECT \* FROM \x60user\x60`).
		WithArgs(email, email, email, 1).
		WillReturnRows(rows)

	user, err := repo.FindByIdentifier(email)
	require.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, uint(1), user.ID)
}

func TestUserRepository_FindByIdentifier_NotFound(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	mock.ExpectQuery(`SELECT \* FROM \x60user\x60`).
		WithArgs("noone", "noone", "noone", 1).
		WillReturnRows(sqlmock.NewRows(nil))

	user, err := repo.FindByIdentifier("noone")
	require.NoError(t, err)
	assert.Nil(t, user)
}

func TestUserRepository_FindByUsername_OnlyMatchesUsername(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	rows := sqlmock.NewRows([]string{
		"id", "created_at", "updated_at", "deleted_at",
		"username", "password", "password_set", "nickname", "email", "phone",
		"site", "avatar_url", "mark", "status", "last_login_at",
	}).AddRow(8, nil, nil, nil, "admin", "hashed", true, nil, "admin@example.com", nil, nil, nil, nil, 1, nil)

	mock.ExpectQuery(`SELECT \* FROM \x60user\x60`).
		WithArgs("admin", 1).
		WillReturnRows(rows)

	user, err := repo.FindByUsername("admin")
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, uint(8), user.ID)
	assert.Equal(t, "admin", user.Username)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_FindByEmail_OnlyMatchesEmail(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	rows := sqlmock.NewRows([]string{
		"id", "created_at", "updated_at", "deleted_at",
		"username", "password", "password_set", "nickname", "email", "phone",
		"site", "avatar_url", "mark", "status", "last_login_at",
	}).AddRow(9, nil, nil, nil, "alice", "hashed", true, nil, "alice@example.com", nil, nil, nil, nil, 1, nil)

	mock.ExpectQuery(`SELECT \* FROM \x60user\x60`).
		WithArgs("alice@example.com", 1).
		WillReturnRows(rows)

	user, err := repo.FindByEmail("alice@example.com")
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, uint(9), user.ID)
	assert.Equal(t, "alice@example.com", *user.Email)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_UpsertSocialLink_RestoresSoftDeletedLink(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "created_at", "updated_at", "deleted_at", "user_id", "platform", "url",
	}).AddRow(12, now, now, now.Add(-time.Hour), 7, "github", "https://github.com/old")

	mock.ExpectQuery(`SELECT \* FROM \x60user_social_link\x60 WHERE user_id = \? AND platform = \? ORDER BY \x60user_social_link\x60\.\x60id\x60 LIMIT \?`).
		WithArgs(uint(7), "github", 1).
		WillReturnRows(rows)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE \x60user_social_link\x60 SET .*deleted_at`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.UpsertSocialLink(7, "github", "https://github.com/new")

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_ExistsByEmail_True(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	rows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery(`SELECT count\(\*\) FROM \x60user\x60`).
		WithArgs("taken@example.com").
		WillReturnRows(rows)

	exists, err := repo.ExistsByEmail("taken@example.com")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestUserRepository_EmailInUseByOther_ChecksMainAndSubEmail(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	mock.ExpectQuery(`SELECT count\(\*\) FROM \x60user\x60`).
		WithArgs("taken@example.com", uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT count\(\*\) FROM \x60user_meta\x60`).
		WithArgs("taken@example.com", uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	exists, err := repo.EmailInUseByOther("taken@example.com", 7)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestUserRepository_FindRolesByUserID(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	rows := sqlmock.NewRows([]string{"name"}).AddRow("ROLE_NORMAL")
	mock.ExpectQuery(`SELECT .+ FROM \x60user_role\x60`).
		WithArgs(uint(1)).
		WillReturnRows(rows)

	roles, err := repo.FindRolesByUserID(1)
	require.NoError(t, err)
	assert.Equal(t, []string{"ROLE_NORMAL"}, roles)
}

func TestUserRepository_FindDetailByID_Found(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	email := "alice@example.com"
	nickname := "Alice"
	avatar := "avatars/alice.png"
	lastLogin := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	birthday := time.Date(1994, 4, 17, 0, 0, 0, 0, time.UTC)
	description := "喜欢写点东西"
	showName := true

	userRows := sqlmock.NewRows([]string{
		"id", "created_at", "updated_at", "deleted_at",
		"username", "password", "password_set", "nickname", "email", "phone",
		"site", "avatar_url", "mark", "status", "last_login_at",
	}).AddRow(7, nil, nil, nil, "alice", "hashed", true, nickname, email, nil, nil, avatar, "注册会员", 1, lastLogin)
	mock.ExpectQuery(`SELECT \* FROM \x60user\x60`).
		WithArgs(uint(7), 1).
		WillReturnRows(userRows)

	roleRows := sqlmock.NewRows([]string{"name"}).AddRow("ROLE_NORMAL").AddRow("ROLE_VIP")
	mock.ExpectQuery(`SELECT .+ FROM \x60user_role\x60`).
		WithArgs(uint(7)).
		WillReturnRows(roleRows)

	metaRows := sqlmock.NewRows([]string{
		"user_id", "name", "description", "sub_email", "gender", "birthday", "id_card",
		"country", "province", "city", "address", "created_at", "updated_at",
	}).AddRow(7, "Alice Wang", description, nil, 1, birthday, nil, "中国", "上海", "上海", "徐汇区", birthday, birthday)
	mock.ExpectQuery(`SELECT \* FROM \x60user_meta\x60`).
		WithArgs(uint(7), 1).
		WillReturnRows(metaRows)

	settingRows := sqlmock.NewRows([]string{
		"user_id", "mail_show", "mail_receive", "dark_mode", "receive_mail",
		"show_name", "show_age", "show_phone", "show_qq", "show_wechat",
		"show_zhihu", "show_sina", "show_bili", "show_position", "created_at", "updated_at",
	}).AddRow(7, 1, 1, 2, true, showName, true, false, false, false, true, false, true, true, birthday, birthday)
	mock.ExpectQuery(`SELECT \* FROM \x60user_setting\x60`).
		WithArgs(uint(7), 1).
		WillReturnRows(settingRows)

	socialRows := sqlmock.NewRows([]string{
		"id", "created_at", "updated_at", "deleted_at", "user_id", "platform", "url",
	}).AddRow(1, birthday, birthday, nil, 7, "github", "https://github.com/alice").
		AddRow(2, birthday, birthday, nil, 7, "zhihu", "https://www.zhihu.com/people/alice")
	mock.ExpectQuery(`SELECT \* FROM \x60user_social_link\x60`).
		WithArgs(uint(7)).
		WillReturnRows(socialRows)

	detail, err := repo.FindDetailByID(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, uint(7), detail.User.ID)
	assert.Equal(t, []string{"ROLE_NORMAL", "ROLE_VIP"}, detail.Roles)
	require.NotNil(t, detail.Meta)
	assert.Equal(t, description, *detail.Meta.Description)
	require.NotNil(t, detail.Setting)
	assert.True(t, detail.Setting.ShowName)
	require.Len(t, detail.SocialLinks, 2)
	assert.Equal(t, "github", detail.SocialLinks[0].Platform)
}

func TestUserRepository_FindDetailByID_NotFound(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	mock.ExpectQuery(`SELECT \* FROM \x60user\x60`).
		WithArgs(uint(99), 1).
		WillReturnRows(sqlmock.NewRows(nil))

	detail, err := repo.FindDetailByID(context.Background(), 99)
	require.NoError(t, err)
	assert.Nil(t, detail)
}

func TestUserRepository_ListLikedContent_ReturnsReplyContext(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	now := time.Date(2026, 6, 25, 10, 24, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT count\(\*\) FROM \(`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT \* FROM \(`).
		WillReturnRows(sqlmock.NewRows([]string{
			"like_id", "liked_at", "kind", "filter_type",
			"content_id", "content_title", "content_excerpt", "content_cover_img_url",
			"parent_kind", "parent_id", "parent_excerpt",
			"root_kind", "root_id", "root_title", "root_excerpt",
			"author_id", "author_username", "author_nickname", "author_avatar_url", "author_site", "author_mark",
			"to_user_id", "to_user_username", "to_user_nickname",
		}).AddRow(
			99, now, user.LikedContentKindReply, user.LikedContentFilterComment,
			88, nil, "对，这里用乐观更新会更顺手", nil,
			user.LikedContentKindComment, 66, "点赞状态最好由服务端返回最终计数",
			user.LikedContentRootArticle, 5, "React Aria 组件实践", "文章摘要",
			12, "ache", "阿澈", "avatars/a.png", nil, "博主",
			2, "vpt", "VPT",
		))

	resp, err := repo.ListLikedContent(user.LikedContentFilter{
		UserID:   7,
		Type:     user.LikedContentFilterComment,
		Page:     1,
		PageSize: 20,
	})

	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	item := resp.Items[0]
	assert.Equal(t, user.LikedContentKindReply, item.Kind)
	assert.Equal(t, user.LikedContentFilterComment, item.Filter)
	assert.Equal(t, uint(12), item.Author.ID)
	assert.Equal(t, "阿澈", *item.Author.Nickname)
	require.NotNil(t, item.Parent)
	assert.Equal(t, uint(66), item.Parent.ID)
	assert.Equal(t, "点赞状态最好由服务端返回最终计数", item.Parent.Excerpt)
	require.NotNil(t, item.ToUser)
	assert.Equal(t, uint(2), item.ToUser.ID)
	assert.Equal(t, "VPT", *item.ToUser.Nickname)
	require.NotNil(t, item.Root)
	assert.Equal(t, user.LikedContentRootArticle, item.Root.Kind)
	assert.Equal(t, "React Aria 组件实践", *item.Root.Title)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_CountLikedContent_UsesVisibleTargets(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	mock.ExpectQuery(`SELECT count\(\*\) FROM \(`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(9))

	count, err := repo.CountLikedContent(7)

	require.NoError(t, err)
	assert.Equal(t, int64(9), count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_Create_Success(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO \x60user\x60`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO \x60user_role\x60`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	nickname := "alice"
	email := "alice@example.com"
	user := &model.User{
		Username:    email,
		Password:    "hashed",
		PasswordSet: true,
		Nickname:    &nickname,
		Email:       &email,
		Status:      1,
	}
	err := repo.Create(user, 3)
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_ListAll_OrdersByRoleNameWeight(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	mock.ExpectQuery(`SELECT COUNT\(DISTINCT\(\x60user\x60\.\x60id\x60\)\) FROM \x60user\x60 LEFT JOIN user_role`).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	rows := sqlmock.NewRows([]string{
		"id", "created_at", "updated_at", "deleted_at",
		"username", "password", "nickname", "email", "phone",
		"site", "avatar_url", "mark", "status", "last_login_at",
	}).AddRow(735, nil, nil, nil, "vip@example.com", "hashed", nil, nil, nil, nil, nil, nil, 1, nil)

	mock.ExpectQuery(`ORDER BY MIN\(CASE role\.name WHEN 'ROLE_ADMIN' THEN 1 WHEN 'ROLE_VIP' THEN 2 WHEN 'ROLE_NORMAL' THEN 3 ELSE 999 END\) ASC, COALESCE\(user\.last_active_at, user\.created_at\) DESC, user\.id DESC`).
		WithArgs(1, 10).
		WillReturnRows(rows)

	status := uint8(1)
	users, total, err := repo.ListAll(user.UserListFilter{Status: &status}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, users, 1)
	assert.Equal(t, uint(735), users[0].ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_ListAll_WithKeywordRoleStatus(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT DISTINCT user\\.\\*").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at",
			"username", "password", "password_set", "nickname", "email", "phone",
			"site", "avatar_url", "mark", "status", "last_login_at",
		}).AddRow(1, nil, nil, nil, "vpt", "hashed", true, "Yevpt", "vpt@example.com", nil, nil, nil, "博主", 1, nil))

	status := uint8(1)
	users, total, err := repo.ListAll(user.UserListFilter{Keyword: "vpt", Role: "ROLE_ADMIN", Status: &status}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, users, 1)
}
