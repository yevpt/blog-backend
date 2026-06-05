package repository_test

import (
	"database/sql"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/repository"
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
	repo := repository.NewUserRepository(db)

	email := "test@example.com"
	rows := sqlmock.NewRows([]string{
		"id", "created_at", "updated_at", "deleted_at",
		"username", "password", "nickname", "email", "phone",
		"site", "avatar_url", "mark", "status", "last_login_at",
	}).AddRow(1, nil, nil, nil, email, "hashed", nil, email, nil, nil, nil, nil, 1, nil)

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
	repo := repository.NewUserRepository(db)

	mock.ExpectQuery(`SELECT \* FROM \x60user\x60`).
		WithArgs("noone", "noone", "noone", 1).
		WillReturnRows(sqlmock.NewRows(nil))

	user, err := repo.FindByIdentifier("noone")
	require.NoError(t, err)
	assert.Nil(t, user)
}

func TestUserRepository_ExistsByEmail_True(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewUserRepository(db)

	rows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery(`SELECT count\(\*\) FROM \x60user\x60`).
		WithArgs("taken@example.com").
		WillReturnRows(rows)

	exists, err := repo.ExistsByEmail("taken@example.com")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestUserRepository_FindRolesByUserID(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewUserRepository(db)

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
	repo := repository.NewUserRepository(db)

	email := "alice@example.com"
	nickname := "Alice"
	avatar := "avatars/alice.png"
	lastLogin := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	birthday := time.Date(1994, 4, 17, 0, 0, 0, 0, time.UTC)
	description := "喜欢写点东西"
	showName := true

	userRows := sqlmock.NewRows([]string{
		"id", "created_at", "updated_at", "deleted_at",
		"username", "password", "nickname", "email", "phone",
		"site", "avatar_url", "mark", "status", "last_login_at",
	}).AddRow(7, nil, nil, nil, "alice", "hashed", nickname, email, nil, nil, avatar, "注册会员", 1, lastLogin)
	mock.ExpectQuery(`SELECT \* FROM \x60user\x60`).
		WithArgs(uint(7), 1).
		WillReturnRows(userRows)

	roleRows := sqlmock.NewRows([]string{"name"}).AddRow("ROLE_NORMAL").AddRow("ROLE_VIP")
	mock.ExpectQuery(`SELECT .+ FROM \x60user_role\x60`).
		WithArgs(uint(7)).
		WillReturnRows(roleRows)

	metaRows := sqlmock.NewRows([]string{
		"user_id", "name", "description", "gender", "birthday", "id_card",
		"country", "province", "city", "address", "created_at", "updated_at",
	}).AddRow(7, "Alice Wang", description, 1, birthday, nil, "中国", "上海", "上海", "徐汇区", birthday, birthday)
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

	detail, err := repo.FindDetailByID(7)
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
	repo := repository.NewUserRepository(db)

	mock.ExpectQuery(`SELECT \* FROM \x60user\x60`).
		WithArgs(uint(99), 1).
		WillReturnRows(sqlmock.NewRows(nil))

	detail, err := repo.FindDetailByID(99)
	require.NoError(t, err)
	assert.Nil(t, detail)
}

func TestUserRepository_Create_Success(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewUserRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO \x60user\x60`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO \x60user_role\x60`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	nickname := "alice"
	email := "alice@example.com"
	user := &model.User{
		Username: email,
		Password: "hashed",
		Nickname: &nickname,
		Email:    &email,
		Status:   1,
	}
	err := repo.Create(user, 3)
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
