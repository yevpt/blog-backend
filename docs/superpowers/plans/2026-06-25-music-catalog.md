# Music Catalog Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade the music module into a lightweight personal music catalog with artists, albums, multi-artist songs, Garage-backed assets, and compatible migration from the existing `music` table.

**Architecture:** Keep Gin handlers thin, place music catalog orchestration and Garage compensation in `internal/service/music`, and keep GORM access in `internal/repository/music`. Use structured DTOs for every API response; never expose `model.*` directly.

**Tech Stack:** Go 1.25+, Gin, GORM/MySQL, Garage/S3 via `pkg/storage`, zap, testify, sqlmock, httptest, swaggo.

---

## Scope Check

This plan covers one cohesive module upgrade: music catalog data model, music admin/public APIs, upload support, and migration. Playlists and public song-list sharing stay out of this implementation; the new model only leaves room for them.

## File Structure

- Modify `internal/model/music.go`: add `MusicArtist`, `MusicAlbum`, `MusicArtistRelation`; extend `Music` with catalog fields while keeping old fields during migration.
- Create `migrations/20260625_music_catalog.sql`: create new tables and backfill existing rows.
- Modify `internal/dto/music.go`: add public/admin DTOs for artists, albums, songs, upload previews, and save requests.
- Modify `internal/repository/music/music.go`: expand repository interface with artist, album, song CRUD and relation replacement.
- Modify `internal/repository/music/music_test.go`: add sqlmock coverage for list/detail/save/delete relation behavior.
- Modify `internal/service/music/music.go`: add public/admin catalog service methods, artist display formatting, save validation, and Garage key resolution.
- Create `internal/service/music/parser.go`: parse old singer strings and artist Chinese names.
- Create `internal/service/music/upload.go`: handle music audio, album cover, and artist avatar uploads through `storage.ObjectStore`.
- Modify `internal/service/music/music_test.go`: add service tests for mapping, validation, parsing, upload compensation.
- Modify `internal/handler/music/music.go`: add public and admin handlers with Swagger comments.
- Modify `internal/handler/music/music_test.go`: add httptest coverage for public/admin endpoints.
- Modify `internal/router/router.go`: wire admin music routes and upload routes.
- Modify `internal/router/router_test.go`: update route construction expectations if needed.
- Modify `cmd/migrate/main.go`: register new models and migrate old music into artist/album/song/relation rows.
- Modify `cmd/migrate/main_test.go`: test singer parsing and music catalog migration planning.
- Run `make swag` after handler changes.

---

### Task 1: Models And SQL Migration

**Files:**
- Modify: `internal/model/music.go`
- Create: `migrations/20260625_music_catalog.sql`

- [ ] **Step 1: Extend models**

Modify `internal/model/music.go` so it contains these catalog models and keeps legacy fields on `Music` for the first release:

```go
type MusicArtist struct {
	Base
	Name        string  `gorm:"size:100;not null;uniqueIndex;comment:歌手名" json:"name"`
	NameZh      *string `gorm:"size:100;comment:中文译名" json:"name_zh"`
	AvatarKey   *string `gorm:"size:500;comment:歌手头像对象 key" json:"avatar_key"`
	Description *string `gorm:"size:500;comment:简介" json:"description"`
}

func (MusicArtist) TableName() string { return "music_artist" }

type MusicAlbum struct {
	Base
	Name        string     `gorm:"size:150;not null;uniqueIndex:idx_music_album_name_artist;comment:专辑名" json:"name"`
	ArtistID    *uint      `gorm:"uniqueIndex:idx_music_album_name_artist;comment:主歌手ID" json:"artist_id"`
	CoverKey    *string    `gorm:"size:500;comment:专辑封面对象 key" json:"cover_key"`
	ReleaseDate *time.Time `gorm:"type:date;comment:发布时间" json:"release_date"`
	Description *string    `gorm:"size:500;comment:简介" json:"description"`
}

func (MusicAlbum) TableName() string { return "music_album" }

type MusicArtistRelation struct {
	ID       uint   `gorm:"primarykey" json:"id"`
	MusicID  uint   `gorm:"not null;uniqueIndex:idx_music_artist_relation;index;comment:音乐ID" json:"music_id"`
	ArtistID uint   `gorm:"not null;uniqueIndex:idx_music_artist_relation;index;comment:歌手ID" json:"artist_id"`
	Role     string `gorm:"size:20;not null;default:primary;uniqueIndex:idx_music_artist_relation;comment:角色" json:"role"`
	Seq      uint   `gorm:"type:int;default:0;comment:展示顺序" json:"seq"`
}

func (MusicArtistRelation) TableName() string { return "music_artist_relation" }
```

Extend `Music` with:

```go
AlbumID           *uint   `gorm:"index;comment:专辑ID" json:"album_id"`
ArtistDisplayName string  `gorm:"size:200;comment:歌手展示名" json:"artist_display_name"`
AlbumTrackNo      uint16  `gorm:"type:smallint unsigned;default:0;comment:专辑序号" json:"album_track_no"`
AudioKey          *string `gorm:"size:500;comment:音频对象 key" json:"audio_key"`
AudioSize         uint64  `gorm:"type:bigint unsigned;default:0;comment:音频大小" json:"audio_size"`
AudioMime         string  `gorm:"size:100;comment:音频 MIME" json:"audio_mime"`
AudioHash         string  `gorm:"size:64;index;comment:音频 hash" json:"audio_hash"`
IsPublic          bool    `gorm:"default:true;index;comment:是否公开" json:"is_public"`
```

- [ ] **Step 2: Create migration SQL**

Create `migrations/20260625_music_catalog.sql` with table creation, new columns, and backfill SQL. Use MySQL 8 syntax and keep old columns:

```sql
CREATE TABLE IF NOT EXISTS `music_artist` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `name` varchar(100) NOT NULL COMMENT '歌手名',
  `name_zh` varchar(100) DEFAULT NULL COMMENT '中文译名',
  `avatar_key` varchar(500) DEFAULT NULL COMMENT '歌手头像对象 key',
  `description` varchar(500) DEFAULT NULL COMMENT '简介',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_music_artist_name` (`name`),
  KEY `idx_music_artist_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `music_album` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `name` varchar(150) NOT NULL COMMENT '专辑名',
  `artist_id` bigint unsigned DEFAULT NULL COMMENT '主歌手ID',
  `cover_key` varchar(500) DEFAULT NULL COMMENT '专辑封面对象 key',
  `release_date` date DEFAULT NULL COMMENT '发布时间',
  `description` varchar(500) DEFAULT NULL COMMENT '简介',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_music_album_name_artist` (`name`, `artist_id`),
  KEY `idx_music_album_deleted_at` (`deleted_at`),
  KEY `idx_music_album_artist_id` (`artist_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `music_artist_relation` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `music_id` bigint unsigned NOT NULL COMMENT '音乐ID',
  `artist_id` bigint unsigned NOT NULL COMMENT '歌手ID',
  `role` varchar(20) NOT NULL DEFAULT 'primary' COMMENT '角色',
  `seq` int unsigned NOT NULL DEFAULT 0 COMMENT '展示顺序',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_music_artist_relation` (`music_id`, `artist_id`, `role`),
  KEY `idx_music_artist_relation_music_id` (`music_id`),
  KEY `idx_music_artist_relation_artist_id` (`artist_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE `music`
  ADD COLUMN `album_id` bigint unsigned DEFAULT NULL COMMENT '专辑ID' AFTER `album`,
  ADD COLUMN `artist_display_name` varchar(200) NOT NULL DEFAULT '' COMMENT '歌手展示名' AFTER `singer`,
  ADD COLUMN `album_track_no` smallint unsigned NOT NULL DEFAULT 0 COMMENT '专辑序号' AFTER `album_id`,
  ADD COLUMN `audio_key` varchar(500) DEFAULT NULL COMMENT '音频对象 key' AFTER `url`,
  ADD COLUMN `audio_size` bigint unsigned NOT NULL DEFAULT 0 COMMENT '音频大小' AFTER `audio_key`,
  ADD COLUMN `audio_mime` varchar(100) NOT NULL DEFAULT '' COMMENT '音频 MIME' AFTER `audio_size`,
  ADD COLUMN `audio_hash` varchar(64) NOT NULL DEFAULT '' COMMENT '音频 hash' AFTER `audio_mime`,
  ADD COLUMN `is_public` tinyint(1) NOT NULL DEFAULT 1 COMMENT '是否公开' AFTER `seq`,
  ADD INDEX `idx_music_album_id` (`album_id`),
  ADD INDEX `idx_music_is_public` (`is_public`),
  ADD INDEX `idx_music_audio_hash` (`audio_hash`);

UPDATE `music`
SET `artist_display_name` = COALESCE(NULLIF(TRIM(`singer`), ''), ''),
    `audio_key` = `url`
WHERE `artist_display_name` = '';
```

Do not try to split all singers with SQL. Detailed backfill belongs in the Go migration task.

- [ ] **Step 3: Format and inspect**

Run: `gofmt -w internal/model/music.go`

Run: `git diff -- internal/model/music.go migrations/20260625_music_catalog.sql`

Expected: diff shows only model/schema additions and legacy fields remain.

- [ ] **Step 4: Commit**

```bash
git add internal/model/music.go migrations/20260625_music_catalog.sql
git commit -m "feat(music): 新增音乐资料库模型"
```

---

### Task 2: DTOs And Parsing Helpers

**Files:**
- Modify: `internal/dto/music.go`
- Create: `internal/service/music/parser.go`
- Test: `internal/service/music/music_test.go`

- [ ] **Step 1: Add parser tests**

Append tests in `internal/service/music/music_test.go`:

```go
func TestSplitArtistNameWithChineseTranslation(t *testing.T) {
	name, nameZh := music.SplitArtistDisplayName("문성남 (文胜南)")

	assert.Equal(t, "문성남", name)
	require.NotNil(t, nameZh)
	assert.Equal(t, "文胜南", *nameZh)
}

func TestSplitArtistNameWithoutChineseTranslation(t *testing.T) {
	name, nameZh := music.SplitArtistDisplayName("Aimer")

	assert.Equal(t, "Aimer", name)
	assert.Nil(t, nameZh)
}

func TestSplitArtistTokensKeepsOrder(t *testing.T) {
	names := music.SplitArtistTokens("Aimer / milet feat. 幾田りら")

	assert.Equal(t, []string{"Aimer", "milet", "幾田りら"}, names)
}
```

- [ ] **Step 2: Run parser tests and verify failure**

Run: `go test ./internal/service/music -run 'TestSplitArtist' -count=1`

Expected: FAIL because parsing helpers are not defined.

- [ ] **Step 3: Implement parser helpers**

Create `internal/service/music/parser.go`:

```go
package music

import (
	"regexp"
	"strings"
)

var artistNameWithZhPattern = regexp.MustCompile(`^(.+?)[\s]*[（(](.+?)[）)]$`)

func SplitArtistDisplayName(value string) (string, *string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	matches := artistNameWithZhPattern.FindStringSubmatch(trimmed)
	if len(matches) != 3 {
		return trimmed, nil
	}
	name := strings.TrimSpace(matches[1])
	nameZh := strings.TrimSpace(matches[2])
	if name == "" || nameZh == "" {
		return trimmed, nil
	}
	return name, &nameZh
}

func SplitArtistTokens(value string) []string {
	replacer := strings.NewReplacer(" feat. ", "/", " feat ", "/", " ft. ", "/", " ft ", "/", "、", "/", "，", "/", ",", "/", "&", "/")
	normalized := replacer.Replace(strings.TrimSpace(value))
	parts := strings.Split(normalized, "/")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	return result
}

func ArtistDisplayName(name string, nameZh *string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if nameZh == nil || strings.TrimSpace(*nameZh) == "" {
		return name
	}
	return name + " (" + strings.TrimSpace(*nameZh) + ")"
}
```

- [ ] **Step 4: Add DTO structs**

Modify `internal/dto/music.go` with concrete request/response types:

```go
type MusicArtistResp struct {
	ID          uint    `json:"id" example:"1"`
	Name        string  `json:"name" example:"문성남"`
	NameZh      *string `json:"name_zh,omitempty" example:"文胜南"`
	DisplayName string  `json:"display_name" example:"문성남 (文胜南)"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
	Description *string `json:"description,omitempty"`
}

type MusicAlbumResp struct {
	ID          uint             `json:"id" example:"1"`
	Name        string           `json:"name" example:"Album"`
	Artist      *MusicArtistResp `json:"artist,omitempty"`
	CoverURL    *string          `json:"cover_url,omitempty"`
	ReleaseDate *string          `json:"release_date,omitempty" example:"2024-01-01"`
	Description *string          `json:"description,omitempty"`
}

type MusicDetailResp struct {
	ID                uint              `json:"id" example:"1"`
	Name              string            `json:"name" example:"Song"`
	ArtistDisplayName string            `json:"artist_display_name" example:"Aimer / milet"`
	Artists           []MusicArtistResp `json:"artists"`
	Album             *MusicAlbumResp   `json:"album,omitempty"`
	AlbumTrackNo      uint16            `json:"album_track_no" example:"1"`
	AudioURL          *string           `json:"audio_url,omitempty"`
	Lyric             *string           `json:"lyric,omitempty"`
	Duration          uint16            `json:"duration" example:"240"`
	IsPublic          bool              `json:"is_public" example:"true"`
	Seq               uint              `json:"seq" example:"0"`
}

type MusicSaveReq struct {
	Name              string `json:"name" binding:"required,max=100"`
	ArtistIDs         []uint `json:"artist_ids" binding:"required,min=1,max=10"`
	ArtistDisplayName string `json:"artist_display_name" binding:"omitempty,max=200"`
	AlbumID           *uint  `json:"album_id"`
	AlbumTrackNo      uint16 `json:"album_track_no"`
	AudioKey          string `json:"audio_key" binding:"required,max=500"`
	Lyric             *string `json:"lyric"`
	Duration          uint16 `json:"duration"`
	IsPublic          bool   `json:"is_public"`
	Seq               uint   `json:"seq"`
}
```

Add request/response DTOs for artist and album save/list in the same file, using concrete types and no `any`.

- [ ] **Step 5: Run music service tests**

Run: `go test ./internal/service/music -run 'TestSplitArtist|TestMusicService_List' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/dto/music.go internal/service/music/parser.go internal/service/music/music_test.go
git commit -m "feat(music): 新增音乐目录 DTO 和解析器"
```

---

### Task 3: Repository Catalog Operations

**Files:**
- Modify: `internal/repository/music/music.go`
- Test: `internal/repository/music/music_test.go`

- [ ] **Step 1: Write failing repository tests**

Add tests that verify public song listing joins albums and artist relations:

```go
func TestMusicRepository_ListPublicSongs_FiltersPublicAndOrders(t *testing.T) {
	db, mock, sqlDB := newMusicMockDB(t)
	defer sqlDB.Close()
	repo := music.NewMusicRepository(db)

	now := time.Now()
	mock.ExpectQuery("SELECT \\* FROM `music` WHERE is_public = \\? AND `music`.`deleted_at` IS NULL ORDER BY seq ASC,id ASC").
		WithArgs(true).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "name", "artist_display_name",
			"album_id", "album_track_no", "audio_key", "lyric", "duration", "audio_size",
			"audio_mime", "audio_hash", "is_public", "seq",
		}).AddRow(1, now, now, nil, "Song", "Aimer", nil, 0, "music/audio/1/a.mp3", nil, 240, 10, "audio/mpeg", "hash", true, 0))

	rows, err := repo.ListPublicSongs()

	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "Song", rows[0].Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}
```

Add tests for `SaveMusic` replacing artist relations inside a transaction:

```go
func TestMusicRepository_SaveMusic_ReplacesArtistRelations(t *testing.T) {
	db, mock, sqlDB := newMusicMockDB(t)
	defer sqlDB.Close()
	repo := music.NewMusicRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `music`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `music_artist_relation` WHERE music_id = \\?").WithArgs(uint(3)).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("INSERT INTO `music_artist_relation`").WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectCommit()

	err := repo.SaveMusic(music.MusicSaveData{
		Music: model.Music{Base: model.Base{ID: 3}, Name: "Song"},
		ArtistRelations: []model.MusicArtistRelation{
			{MusicID: 3, ArtistID: 1, Role: "primary", Seq: 0},
			{MusicID: 3, ArtistID: 2, Role: "primary", Seq: 1},
		},
	})

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
```

- [ ] **Step 2: Run repository tests and verify failure**

Run: `go test ./internal/repository/music -run 'TestMusicRepository_ListPublicSongs|TestMusicRepository_SaveMusic' -count=1`

Expected: FAIL because methods and save data type do not exist.

- [ ] **Step 3: Implement repository methods**

Expand `internal/repository/music/music.go`:

```go
type MusicSaveData struct {
	Music           model.Music
	ArtistRelations []model.MusicArtistRelation
}

type MusicRepository interface {
	List() ([]model.Music, error)
	ListPublicSongs() ([]model.Music, error)
	FindMusic(id uint) (*model.Music, error)
	SaveMusic(data MusicSaveData) error
	DeleteMusic(id uint) error
	ListArtists(keyword string) ([]model.MusicArtist, error)
	FindArtists(ids []uint) ([]model.MusicArtist, error)
	SaveArtist(artist model.MusicArtist) (*model.MusicArtist, error)
	DeleteArtist(id uint) error
	ListAlbums(keyword string) ([]model.MusicAlbum, error)
	FindAlbum(id uint) (*model.MusicAlbum, error)
	SaveAlbum(album model.MusicAlbum) (*model.MusicAlbum, error)
	DeleteAlbum(id uint) error
	MusicArtistRelations(musicIDs []uint) (map[uint][]model.MusicArtist, error)
}
```

Implementation rules:

- Public song listing uses `WHERE is_public = true`.
- Soft delete uses GORM `Delete`.
- `SaveMusic` uses `db.Transaction`.
- Relation replacement deletes by `music_id`, then creates ordered relation rows.
- Repository returns `model.*`, never `dto.*`.

- [ ] **Step 4: Run repository package**

Run: `go test ./internal/repository/music -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/music/music.go internal/repository/music/music_test.go
git commit -m "feat(music): 补全音乐资料库仓储"
```

---

### Task 4: Music Service Behavior

**Files:**
- Modify: `internal/service/music/music.go`
- Create: `internal/service/music/upload.go`
- Test: `internal/service/music/music_test.go`

- [ ] **Step 1: Write service mapping tests**

Add tests:

```go
func TestMusicService_ListPublic_ResolvesAudioAndArtistDisplay(t *testing.T) {
	audioKey := "music/audio/1/hash.mp3"
	repo := &stubMusicRepository{
		rows: []model.Music{{
			Base: model.Base{ID: 1},
			Name: "Song",
			ArtistDisplayName: "문성남 (文胜南)",
			AudioKey: &audioKey,
			Duration: 180,
			IsPublic: true,
		}},
	}
	svc := music.NewMusicService(repo, stubMusicResolver{})

	resp, err := svc.ListPublic()

	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	assert.Equal(t, "https://cdn.example.com/music/audio/1/hash.mp3", *resp.List[0].AudioURL)
	assert.Equal(t, "문성남 (文胜南)", resp.List[0].ArtistDisplayName)
}

func TestMusicService_SaveMusic_RejectsMissingArtists(t *testing.T) {
	repo := &stubMusicRepository{artists: []model.MusicArtist{{Base: model.Base{ID: 1}, Name: "Aimer"}}}
	svc := music.NewMusicService(repo, nil)

	err := svc.SaveMusic(dto.MusicSaveReq{Name: "Song", ArtistIDs: []uint{1, 2}, AudioKey: "music/audio/1/a.mp3"})

	require.ErrorIs(t, err, music.ErrMusicArtistNotFound)
}
```

- [ ] **Step 2: Run service tests and verify failure**

Run: `go test ./internal/service/music -run 'TestMusicService_ListPublic|TestMusicService_SaveMusic' -count=1`

Expected: FAIL because methods and errors do not exist.

- [ ] **Step 3: Implement service interface**

Modify `internal/service/music/music.go`:

```go
var (
	ErrMusicNotFound       = errors.New("音乐不存在")
	ErrMusicArtistNotFound = errors.New("歌手不存在")
	ErrMusicAlbumNotFound  = errors.New("专辑不存在")
	ErrMusicUploadInvalid  = errors.New("音乐文件无效")
)

type MusicService interface {
	List() (*dto.MusicListResp, error)
	ListPublic() (*dto.MusicListResp, error)
	GetPublicDetail(id uint) (*dto.MusicDetailResp, error)
	ListAdmin(query dto.MusicAdminListReq) (*dto.MusicAdminListResp, error)
	SaveMusic(req dto.MusicSaveReq) error
	DeleteMusic(id uint) error
	ListArtists(keyword string) ([]dto.MusicArtistResp, error)
	SaveArtist(req dto.MusicArtistSaveReq) (*dto.MusicArtistResp, error)
	DeleteArtist(id uint) error
	ListAlbums(keyword string) ([]dto.MusicAlbumResp, error)
	SaveAlbum(req dto.MusicAlbumSaveReq) (*dto.MusicAlbumResp, error)
	DeleteAlbum(id uint) error
}
```

Keep `List()` as a compatibility wrapper calling `ListPublic()` until handlers are updated.

Implement:

- `artistDTO` uses `ArtistDisplayName`.
- `musicDTO` resolves `AudioKey` with `storage.ResolvePtrURL`.
- `SaveMusic` verifies all `ArtistIDs` exist.
- `SaveMusic` verifies `AlbumID` exists when provided.
- `SaveMusic` builds relation rows in request order.
- If `ArtistDisplayName` is empty, derive it by joining selected artist display names with ` / `.

- [ ] **Step 4: Run service package**

Run: `go test ./internal/service/music -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/music internal/dto/music.go
git commit -m "feat(music): 实现音乐资料库服务"
```

---

### Task 5: HTTP API And Routes

**Files:**
- Modify: `internal/handler/music/music.go`
- Modify: `internal/handler/music/music_test.go`
- Modify: `internal/router/router.go`
- Modify: `internal/router/router_test.go`

- [ ] **Step 1: Write handler tests**

Add tests for admin save and public detail:

```go
func TestMusicHandler_GetPublicDetail_Success(t *testing.T) {
	svc := &stubMusicService{detail: &dto.MusicDetailResp{ID: 1, Name: "Song"}}
	r := newMusicRouter(svc)
	r.GET("/music/:id", music.NewMusicHandler(svc).GetPublicDetail)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/music/1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMusicHandler_SaveMusic_BadJSON(t *testing.T) {
	svc := &stubMusicService{}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := music.NewMusicHandler(svc)
	r.POST("/admin/music", h.SaveMusic)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/music", strings.NewReader("{"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
```

- [ ] **Step 2: Run handler tests and verify failure**

Run: `go test ./internal/handler/music -run 'TestMusicHandler_GetPublicDetail|TestMusicHandler_SaveMusic' -count=1`

Expected: FAIL because handler methods do not exist.

- [ ] **Step 3: Implement handlers**

Add methods with Swagger comments:

- `List`
- `GetPublicDetail`
- `ListArtists`
- `GetArtist`
- `ListAlbums`
- `GetAlbum`
- `ListAdmin`
- `SaveMusic`
- `DeleteMusic`
- `ListAdminArtists`
- `SaveArtist`
- `DeleteArtist`
- `ListAdminAlbums`
- `SaveAlbum`
- `DeleteAlbum`

Handler rules:

- Bind JSON/query only.
- Map binding errors to `response.Fail(c, response.CodeBadRequest, "参数错误")`.
- Map not-found service errors to `response.NotFound(c)`.
- Use `response.Success` / `response.Fail`; do not use `c.JSON`.

- [ ] **Step 4: Register routes**

Modify `internal/router/router.go`:

```go
r.GET("/music", handlers.music.List)
r.GET("/music/:id", handlers.music.GetPublicDetail)
r.GET("/music/artists", handlers.music.ListArtists)
r.GET("/music/albums", handlers.music.ListAlbums)
```

Inside admin group:

```go
admin.GET("/music", handlers.music.ListAdmin)
admin.POST("/music", handlers.music.SaveMusic)
admin.PUT("/music/:id", handlers.music.SaveMusic)
admin.DELETE("/music/:id", handlers.music.DeleteMusic)
admin.GET("/music/artists", handlers.music.ListAdminArtists)
admin.POST("/music/artists", handlers.music.SaveArtist)
admin.PUT("/music/artists/:id", handlers.music.SaveArtist)
admin.DELETE("/music/artists/:id", handlers.music.DeleteArtist)
admin.GET("/music/albums", handlers.music.ListAdminAlbums)
admin.POST("/music/albums", handlers.music.SaveAlbum)
admin.PUT("/music/albums/:id", handlers.music.SaveAlbum)
admin.DELETE("/music/albums/:id", handlers.music.DeleteAlbum)
```

- [ ] **Step 5: Run handler/router tests**

Run: `go test ./internal/handler/music ./internal/router -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/handler/music internal/router/router.go internal/router/router_test.go
git commit -m "feat(music): 注册音乐资料库接口"
```

---

### Task 6: Upload Service

**Files:**
- Create: `pkg/audiofile/audiofile.go`
- Create: `pkg/audiofile/audiofile_test.go`
- Modify: `internal/service/music/upload.go`
- Modify: `internal/handler/music/music.go`
- Modify: `internal/router/router.go`
- Test: `internal/service/music/music_test.go`
- Test: `internal/handler/music/music_test.go`

- [ ] **Step 1: Add audio validation tests**

Create `pkg/audiofile/audiofile_test.go`:

```go
package audiofile_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/audiofile"
)

func TestValidateMP3ByID3Header(t *testing.T) {
	data := append([]byte("ID3\x04\x00\x00\x00\x00\x00\x00"), []byte("payload")...)

	result, err := audiofile.Validate("song.mp3", data, 1024)

	require.NoError(t, err)
	assert.Equal(t, "audio/mpeg", result.ContentType)
	assert.Equal(t, ".mp3", result.Ext)
	assert.NotEmpty(t, result.SHA256)
}

func TestValidateRejectsInvalidAudio(t *testing.T) {
	_, err := audiofile.Validate("song.txt", []byte("not audio"), 1024)

	require.ErrorIs(t, err, audiofile.ErrInvalidAudio)
}
```

- [ ] **Step 2: Run audio tests and verify failure**

Run: `go test ./pkg/audiofile -count=1`

Expected: FAIL because package does not exist.

- [ ] **Step 3: Implement minimal audio validator**

Create `pkg/audiofile/audiofile.go`:

```go
package audiofile

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

var (
	ErrInvalidAudio = errors.New("音频文件无效")
	ErrAudioTooLarge = errors.New("音频文件过大")
)

type Result struct {
	Data        []byte
	ContentType string
	Ext         string
	SHA256      string
	Size        uint64
}

func Validate(name string, data []byte, maxBytes int) (Result, error) {
	if maxBytes > 0 && len(data) > maxBytes {
		return Result{}, ErrAudioTooLarge
	}
	contentType, ext := detectAudio(name, data)
	if contentType == "" {
		return Result{}, ErrInvalidAudio
	}
	sum := sha256.Sum256(data)
	return Result{
		Data: data,
		ContentType: contentType,
		Ext: ext,
		SHA256: hex.EncodeToString(sum[:]),
		Size: uint64(len(data)),
	}, nil
}

func detectAudio(name string, data []byte) (string, string) {
	lower := strings.ToLower(name)
	if len(data) >= 3 && string(data[:3]) == "ID3" {
		return "audio/mpeg", ".mp3"
	}
	if len(data) >= 2 && data[0] == 0xff && data[1]&0xe0 == 0xe0 {
		return "audio/mpeg", ".mp3"
	}
	if len(data) >= 12 && string(data[4:8]) == "ftyp" && (strings.HasSuffix(lower, ".m4a") || strings.HasSuffix(lower, ".mp4")) {
		return "audio/mp4", ".m4a"
	}
	if len(data) >= 4 && string(data[:4]) == "fLaC" {
		return "audio/flac", ".flac"
	}
	return "", ""
}
```

Metadata parsing can be added behind this package later without changing service contracts. For this task, the upload API stores safe asset metadata and leaves editable song metadata to the save request.

- [ ] **Step 4: Implement music upload methods**

In `internal/service/music/upload.go`, add:

```go
const MaxMusicAudioBytes = 50 * 1024 * 1024

type MusicAudioUploadInput struct {
	UserID uint
	Name   string
	Data   []byte
}

func (s *musicService) UploadAudio(ctx context.Context, input MusicAudioUploadInput) (*dto.MusicUploadResp, error) {
	result, err := audiofile.Validate(input.Name, input.Data, MaxMusicAudioBytes)
	if err != nil {
		return nil, ErrMusicUploadInvalid
	}
	key := fmt.Sprintf("temp/music/%d/audio/%s%s", input.UserID, result.SHA256, result.Ext)
	exists, err := s.store.ObjectExists(ctx, key)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := s.store.PutObject(ctx, key, result.Data, result.ContentType); err != nil {
			return nil, err
		}
	}
	url, err := s.store.ObjectURL(ctx, key)
	if err != nil {
		return nil, err
	}
	return &dto.MusicUploadResp{
		Key: key,
		URL: url,
		Size: result.Size,
		Mime: result.ContentType,
		Hash: result.SHA256,
	}, nil
}
```

Add similar methods for album cover and artist avatar by reusing `pkg/imagefile.Validate`, with keys:

```text
temp/music/{user_id}/album-cover/{md5}{ext}
temp/music/{user_id}/artist-avatar/{md5}{ext}
```

- [ ] **Step 5: Add upload handlers**

Add handler methods:

- `UploadAudio`
- `UploadAlbumCover`
- `UploadArtistAvatar`

Each reads multipart file with `io.LimitReader`, extracts `jwt.GetClaims(c)`, calls service, and maps invalid file errors to `response.Fail`.

Register admin routes:

```go
admin.POST("/music/uploads/audio", handlers.music.UploadAudio)
admin.POST("/music/uploads/album-cover", handlers.music.UploadAlbumCover)
admin.POST("/music/uploads/artist-avatar", handlers.music.UploadArtistAvatar)
```

Apply an existing upload rate limiter if one fits; otherwise add a dedicated admin upload limiter in the same style as `RateLimitTempUpload`.

- [ ] **Step 6: Run upload tests**

Run: `go test ./pkg/audiofile ./internal/service/music ./internal/handler/music -run 'Upload|Validate' -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/audiofile internal/service/music internal/handler/music internal/router/router.go
git commit -m "feat(music): 新增音乐资源上传"
```

---

### Task 7: Migration Tool Updates

**Files:**
- Modify: `cmd/migrate/main.go`
- Modify: `cmd/migrate/main_test.go`

- [ ] **Step 1: Write migration helper tests**

Add tests in `cmd/migrate/main_test.go`:

```go
func TestBuildMusicArtistSeeds_SplitsChineseTranslation(t *testing.T) {
	seeds := buildMusicArtistSeeds("문성남 (文胜南)")

	require.Len(t, seeds, 1)
	assert.Equal(t, "문성남", seeds[0].Name)
	require.NotNil(t, seeds[0].NameZh)
	assert.Equal(t, "文胜南", *seeds[0].NameZh)
}

func TestBuildMusicArtistSeeds_SplitsCollaboration(t *testing.T) {
	seeds := buildMusicArtistSeeds("Aimer / milet feat. 幾田りら")

	assert.Equal(t, []string{"Aimer", "milet", "幾田りら"}, []string{seeds[0].Name, seeds[1].Name, seeds[2].Name})
}
```

- [ ] **Step 2: Run migration tests and verify failure**

Run: `go test ./cmd/migrate -run 'TestBuildMusicArtistSeeds' -count=1`

Expected: FAIL because helper does not exist.

- [ ] **Step 3: Register new models**

Modify `autoMigrate` in `cmd/migrate/main.go` to include:

```go
&model.MusicArtist{},
&model.MusicAlbum{},
&model.MusicArtistRelation{},
```

Place them before `&model.Music{}`.

- [ ] **Step 4: Update `migrateMusic`**

Change `migrateMusic` to:

- Parse singer into artist seeds.
- `FirstOrCreate` each `model.MusicArtist` by `name`.
- Create or find album by `name + artist_id`.
- Create `model.Music` with:
  - `ArtistDisplayName = singer.String`
  - `AlbumID = &album.ID` when album exists
  - `AudioKey = nullStr(url)`
  - legacy `URL = nullStr(url)`
  - legacy `CoverImgUrl = nullStr(icon)`
- Create `model.MusicArtistRelation` rows in order.

Use helper type:

```go
type musicArtistSeed struct {
	Name   string
	NameZh *string
}
```

Migration should keep existing `ID` values for `music`, preserving `article_music.music_id`.

- [ ] **Step 5: Run migration tests**

Run: `go test ./cmd/migrate -run 'TestBuildMusicArtistSeeds|TestBuildMomentMediaGaragePlan|TestBuildArticleGaragePlan' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/migrate/main.go cmd/migrate/main_test.go
git commit -m "feat(music): 更新音乐迁移工具"
```

---

### Task 8: Swagger And Final Verification

**Files:**
- Modify: `docs/docs.go`
- Modify: `docs/swagger.json`
- Modify: `docs/swagger.yaml`

- [ ] **Step 1: Generate Swagger**

Run: `make swag`

Expected: command exits 0 and Swagger docs contain `/music`, `/music/{id}`, `/admin/music`, and `/admin/music/uploads/audio`.

- [ ] **Step 2: Run targeted tests**

Run:

```bash
go test ./pkg/audiofile ./pkg/storage ./internal/repository/music ./internal/service/music ./internal/handler/music ./internal/router ./cmd/migrate -count=1
```

Expected: PASS.

- [ ] **Step 3: Run related article tests**

Run:

```bash
go test ./internal/repository/article ./internal/service/article ./internal/handler/article -count=1
```

Expected: PASS; article `music_ids` behavior remains compatible.

- [ ] **Step 4: Inspect git diff**

Run:

```bash
git diff --stat
git diff -- migrations/20260625_music_catalog.sql internal/model/music.go internal/router/router.go
```

Expected: only music catalog, migration, Swagger, and related tests changed.

- [ ] **Step 5: Commit verification artifacts**

```bash
git add docs docs/swagger.json docs/swagger.yaml
git commit -m "docs(swagger): 更新音乐资料库接口文档"
```

If Swagger files were already committed with the handler task, skip this commit and note that `make swag` produced no diff.

## Self-Review

- Spec coverage: model split, Chinese translation display, multi-artist songs, Garage paths, upload endpoints, migration locations, and validation strategy are each mapped to tasks.
- Red-flag scan: every task names concrete files and commands, with no deferred work markers.
- Type consistency: DTO, model, repository, service, and handler names use `MusicArtist`, `MusicAlbum`, `MusicArtistRelation`, `MusicSaveReq`, and `MusicDetailResp` consistently.
