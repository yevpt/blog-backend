# Music Catalog Cursor-Ready Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade the existing music module into a lightweight personal music catalog with artists, albums, multi-artist songs, Garage-backed assets, compatible old-data migration, and article background-music support.

**Architecture:** Keep HTTP binding in `internal/handler/music`, business validation and Garage compensation in `internal/service/music`, and GORM persistence in `internal/repository/music`. Keep old `music` columns during the first release so migration and rollback stay safe.

**Tech Stack:** Go 1.25+, Gin, GORM/MySQL, Garage/S3 via `pkg/storage`, testify, sqlmock, httptest, swaggo.

---

## Cursor Auto Execution Rules

Use this plan one task at a time. Do not ask Cursor Auto to implement the whole plan in one run.

Hard rules for every task:

- Only edit files listed in the task.
- Do not remove legacy `music.singer`, `music.album`, `music.url`, or `music.cover_img_url` in this implementation.
- Do not expose `model.*` in DTOs or Swagger.
- Do not use package-level globals for DB, storage, logger, or service instances.
- Do not add unrelated refactors.
- Do not add AI signatures to commits.
- After each task, run the exact test command listed in that task.
- Stop after the task is complete and report changed files, tests run, and remaining failures.

Known scope boundaries:

- First phase does **not** parse ID3 metadata beyond safe file type/hash/size. Upload returns asset metadata; song title/artist/album are confirmed through save APIs.
- First phase does **not** build playlists.
- First phase does **not** delete old Garage keys after migration. It copies new keys and leaves cleanup for a later task.

## File Ownership Map

- `internal/model/music.go`: GORM schema only.
- `migrations/20260625_music_catalog.sql`: existing database upgrade SQL only.
- `internal/dto/music.go`: all request and response structs for music APIs.
- `internal/repository/music/music.go`: DB queries and transactions; returns `model.*`.
- `internal/service/music/parser.go`: artist name splitting and display-name helpers.
- `internal/service/music/music.go`: catalog business logic and DTO mapping.
- `internal/service/music/upload.go`: Garage upload orchestration for music assets.
- `internal/handler/music/music.go`: Gin handlers and Swagger annotations.
- `internal/router/router.go`: route registration only.
- `pkg/audiofile/audiofile.go`: minimal audio byte validation.
- `cmd/migrate/main.go`: old-blog migration tool updates.

---

## Task 0: Preflight

**Files:** none

- [ ] **Step 1: Read required context**

Read:

```text
AGENTS.md
.agents/skills/go-layering/SKILL.md
.agents/skills/http-api/SKILL.md
.agents/skills/go-testing/SKILL.md
docs/superpowers/specs/2026-06-25-music-catalog-design.md
```

- [ ] **Step 2: Inspect current module**

Run:

```bash
sed -n '1,220p' internal/model/music.go
sed -n '1,220p' internal/dto/music.go
sed -n '1,260p' internal/service/music/music.go
sed -n '1,260p' internal/repository/music/music.go
sed -n '292,440p' internal/router/router.go
```

Expected: current module has only public `GET /music`, simple `music` table, and no artist/album tables.

- [ ] **Step 3: Verify baseline tests**

Run:

```bash
go test ./internal/repository/music ./internal/service/music ./internal/handler/music -count=1
```

Expected: PASS before implementation. If baseline fails, stop and report.

---

## Task 1: Add Catalog Models And SQL Migration

**Files:**
- Modify: `internal/model/music.go`
- Create: `migrations/20260625_music_catalog.sql`

- [ ] **Step 1: Modify `internal/model/music.go`**

Keep existing `Music` and `ArticleMusic`. Add these structs above `Music`:

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

Extend existing `Music` with these fields, but do not delete old fields:

```go
AlbumID           *uint  `gorm:"index;comment:专辑ID" json:"album_id"`
ArtistDisplayName string `gorm:"size:200;comment:歌手展示名" json:"artist_display_name"`
AlbumTrackNo      uint16 `gorm:"type:smallint unsigned;default:0;comment:专辑序号" json:"album_track_no"`
AudioKey          *string `gorm:"size:500;comment:音频对象 key" json:"audio_key"`
AudioSize         uint64  `gorm:"type:bigint unsigned;default:0;comment:音频大小" json:"audio_size"`
AudioMime         string  `gorm:"size:100;comment:音频 MIME" json:"audio_mime"`
AudioHash         string  `gorm:"size:64;index;comment:音频 hash" json:"audio_hash"`
IsPublic          bool    `gorm:"default:true;index;comment:是否公开" json:"is_public"`
```

Place `AlbumID` after legacy `Album`, `ArtistDisplayName` after `Singer`, and `AudioKey` after legacy `URL` so the model remains readable.

- [ ] **Step 2: Create SQL migration**

Create `migrations/20260625_music_catalog.sql` with exactly these operations:

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

Do not add SQL logic to split singers. That is handled by Go migration code later.

- [ ] **Step 3: Format and test**

Run:

```bash
gofmt -w internal/model/music.go
go test ./internal/model ./internal/repository/music -count=1
```

Expected: `./internal/model` can report no test files; repository music tests must PASS.

- [ ] **Step 4: Stop and report**

Report:

```text
Task 1 complete.
Changed files:
- internal/model/music.go
- migrations/20260625_music_catalog.sql
Tests:
- go test ./internal/model ./internal/repository/music -count=1
```

---

## Task 2: Add Parser Helpers

**Files:**
- Create: `internal/service/music/parser.go`
- Modify: `internal/service/music/music_test.go`

- [ ] **Step 1: Add tests**

Append to `internal/service/music/music_test.go`:

```go
func TestSplitArtistDisplayName_WithChineseTranslation(t *testing.T) {
	name, nameZh := music.SplitArtistDisplayName("문성남 (文胜南)")

	assert.Equal(t, "문성남", name)
	require.NotNil(t, nameZh)
	assert.Equal(t, "文胜南", *nameZh)
}

func TestSplitArtistDisplayName_WithFullWidthParentheses(t *testing.T) {
	name, nameZh := music.SplitArtistDisplayName("문성남（文胜南）")

	assert.Equal(t, "문성남", name)
	require.NotNil(t, nameZh)
	assert.Equal(t, "文胜南", *nameZh)
}

func TestSplitArtistDisplayName_WithoutChineseTranslation(t *testing.T) {
	name, nameZh := music.SplitArtistDisplayName("Aimer")

	assert.Equal(t, "Aimer", name)
	assert.Nil(t, nameZh)
}

func TestSplitArtistTokens_KeepsOrderAndDeduplicates(t *testing.T) {
	names := music.SplitArtistTokens("Aimer / milet feat. 幾田りら / Aimer")

	assert.Equal(t, []string{"Aimer", "milet", "幾田りら"}, names)
}

func TestArtistDisplayName_UsesChineseTranslation(t *testing.T) {
	nameZh := "文胜南"

	assert.Equal(t, "문성남 (文胜南)", music.ArtistDisplayName("문성남", &nameZh))
}
```

- [ ] **Step 2: Verify tests fail**

Run:

```bash
go test ./internal/service/music -run 'TestSplitArtist|TestArtistDisplayName' -count=1
```

Expected: FAIL because helpers do not exist.

- [ ] **Step 3: Implement parser**

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
	replacer := strings.NewReplacer(
		" feat. ", "/",
		" feat ", "/",
		" ft. ", "/",
		" ft ", "/",
		"、", "/",
		"，", "/",
		",", "/",
		"&", "/",
	)
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

- [ ] **Step 4: Test**

Run:

```bash
gofmt -w internal/service/music/parser.go internal/service/music/music_test.go
go test ./internal/service/music -run 'TestSplitArtist|TestArtistDisplayName|TestMusicService_List' -count=1
```

Expected: PASS.

- [ ] **Step 5: Stop and report**

Report changed files and test result.

---

## Task 3: Replace Music DTOs With Explicit Catalog DTOs

**Files:**
- Modify: `internal/dto/music.go`

- [ ] **Step 1: Replace file content**

Replace `internal/dto/music.go` with:

```go
package dto

type MusicArtistResp struct {
	ID          uint    `json:"id" example:"1"`
	Name        string  `json:"name" example:"문성남"`
	NameZh      *string `json:"name_zh,omitempty" example:"文胜南"`
	DisplayName string  `json:"display_name" example:"문성남 (文胜南)"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
	Description *string `json:"description,omitempty"`
}

type MusicArtistSaveReq struct {
	ID          uint    `json:"id"`
	Name        string  `json:"name" binding:"required,max=100"`
	NameZh      *string `json:"name_zh" binding:"omitempty,max=100"`
	AvatarKey   *string `json:"avatar_key" binding:"omitempty,max=500"`
	Description *string `json:"description" binding:"omitempty,max=500"`
}

type MusicArtistListResp struct {
	List []MusicArtistResp `json:"list"`
}

type MusicAlbumResp struct {
	ID          uint             `json:"id" example:"1"`
	Name        string           `json:"name" example:"Album"`
	Artist      *MusicArtistResp `json:"artist,omitempty"`
	CoverURL    *string          `json:"cover_url,omitempty"`
	ReleaseDate *string          `json:"release_date,omitempty" example:"2024-01-01"`
	Description *string          `json:"description,omitempty"`
}

type MusicAlbumSaveReq struct {
	ID          uint    `json:"id"`
	Name        string  `json:"name" binding:"required,max=150"`
	ArtistID    *uint   `json:"artist_id"`
	CoverKey    *string `json:"cover_key" binding:"omitempty,max=500"`
	ReleaseDate *string `json:"release_date" binding:"omitempty"`
	Description *string `json:"description" binding:"omitempty,max=500"`
}

type MusicAlbumListResp struct {
	List []MusicAlbumResp `json:"list"`
}

type MusicItemResp struct {
	ID                uint              `json:"id" example:"1"`
	Name              string            `json:"name" example:"Song"`
	ArtistDisplayName string            `json:"artist_display_name" example:"Aimer / milet"`
	Artists           []MusicArtistResp `json:"artists"`
	Album             *MusicAlbumResp   `json:"album,omitempty"`
	AlbumTrackNo      uint16            `json:"album_track_no" example:"1"`
	AudioURL          *string           `json:"audio_url,omitempty"`
	CoverURL          *string           `json:"cover_url,omitempty"`
	Duration          uint16            `json:"duration" example:"240"`
	IsPublic          bool              `json:"is_public" example:"true"`
	Seq               uint              `json:"seq" example:"0"`
}

type MusicDetailResp struct {
	MusicItemResp
	Lyric *string `json:"lyric,omitempty"`
}

type MusicListResp struct {
	List []MusicItemResp `json:"list"`
}

type MusicAdminListReq struct {
	Keyword  string `form:"keyword"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

type MusicAdminListResp struct {
	List  []MusicItemResp `json:"list"`
	Total int64           `json:"total"`
}

type MusicSaveReq struct {
	ID                uint    `json:"id"`
	Name              string  `json:"name" binding:"required,max=100"`
	ArtistIDs         []uint  `json:"artist_ids" binding:"required,min=1,max=10"`
	ArtistDisplayName string  `json:"artist_display_name" binding:"omitempty,max=200"`
	AlbumID           *uint   `json:"album_id"`
	AlbumTrackNo      uint16  `json:"album_track_no"`
	AudioKey          string  `json:"audio_key" binding:"required,max=500"`
	Lyric             *string `json:"lyric"`
	Duration          uint16  `json:"duration"`
	IsPublic          bool    `json:"is_public"`
	Seq               uint    `json:"seq"`
}

type MusicUploadResp struct {
	Key  string `json:"key" example:"temp/music/1/audio/hash.mp3"`
	URL  string `json:"url" example:"https://cdn.example.com/music/audio/hash.mp3"`
	Size uint64 `json:"size" example:"123456"`
	Mime string `json:"mime" example:"audio/mpeg"`
	Hash string `json:"hash" example:"abcdef"`
}
```

- [ ] **Step 2: Test compile**

Run:

```bash
gofmt -w internal/dto/music.go
go test ./internal/service/music ./internal/handler/music -count=1
```

Expected: this can FAIL because service/handler still reference old fields (`URL`, `CoverImgUrl`, `Album`, `Singer`). Continue only when failures are limited to those known downstream references. Fix syntax errors or missing DTO names before stopping.

- [ ] **Step 3: Stop and report**

Report changed file and compile errors if present. Do not modify service or handler in this task.

---

## Task 4: Repository Interfaces And Read Queries

**Files:**
- Modify: `internal/repository/music/music.go`
- Modify: `internal/repository/music/music_test.go`

- [ ] **Step 1: Add repository tests**

In `internal/repository/music/music_test.go`, add:

```go
func TestMusicRepository_ListPublicSongs_FiltersPublicAndOrders(t *testing.T) {
	db, mock, sqlDB := newMusicMockDB(t)
	defer sqlDB.Close()
	repo := music.NewMusicRepository(db)

	now := time.Now()
	mock.ExpectQuery("SELECT \\* FROM `music` WHERE is_public = \\? AND `music`.`deleted_at` IS NULL ORDER BY seq ASC,id ASC").
		WithArgs(true).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "name", "singer", "artist_display_name",
			"album", "album_id", "album_track_no", "song_date", "url", "audio_key", "audio_size",
			"audio_mime", "audio_hash", "cover_img_url", "description", "lyric", "duration", "seq", "is_public",
		}).AddRow(1, now, now, nil, "Song", "Singer", "Singer", "Album", nil, 0, nil, nil, "music/audio/1/a.mp3", 12, "audio/mpeg", "hash", nil, nil, nil, 240, 0, true))

	rows, err := repo.ListPublicSongs()

	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "Song", rows[0].Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMusicRepository_MusicArtistRelations_LoadsArtists(t *testing.T) {
	db, mock, sqlDB := newMusicMockDB(t)
	defer sqlDB.Close()
	repo := music.NewMusicRepository(db)

	now := time.Now()
	mock.ExpectQuery("SELECT music_artist_relation.music_id, music_artist.\\* FROM `music_artist_relation` JOIN music_artist ON music_artist.id = music_artist_relation.artist_id AND music_artist.deleted_at IS NULL").
		WillReturnRows(sqlmock.NewRows([]string{
			"music_id", "id", "created_at", "updated_at", "deleted_at", "name", "name_zh", "avatar_key", "description",
		}).AddRow(1, 2, now, now, nil, "Aimer", nil, nil, nil))

	rows, err := repo.MusicArtistRelations([]uint{1})

	require.NoError(t, err)
	require.Len(t, rows[1], 1)
	assert.Equal(t, "Aimer", rows[1][0].Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}
```

- [ ] **Step 2: Verify tests fail**

Run:

```bash
go test ./internal/repository/music -run 'ListPublicSongs|MusicArtistRelations' -count=1
```

Expected: FAIL because methods do not exist.

- [ ] **Step 3: Implement read repository contract**

Modify `internal/repository/music/music.go`:

```go
type MusicRepository interface {
	List() ([]model.Music, error)
	ListPublicSongs() ([]model.Music, error)
	FindMusic(id uint) (*model.Music, error)
	MusicArtistRelations(musicIDs []uint) (map[uint][]model.MusicArtist, error)
	ListArtists(keyword string) ([]model.MusicArtist, error)
	FindArtists(ids []uint) ([]model.MusicArtist, error)
	ListAlbums(keyword string) ([]model.MusicAlbum, error)
	FindAlbum(id uint) (*model.MusicAlbum, error)
}
```

Keep existing `List()` as a compatibility wrapper and make it call `ListPublicSongs()`.

Implement:

- `ListPublicSongs`: `WHERE is_public = true`, order `seq ASC, id ASC`.
- `FindMusic`: first by ID; return `nil, nil` on `gorm.ErrRecordNotFound`.
- `MusicArtistRelations`: join relation to artists, order relation `seq ASC`.
- `ListArtists`: optional keyword on `name OR name_zh`, order `id DESC`.
- `FindArtists`: load artists by IDs.
- `ListAlbums`: optional keyword on name, order `id DESC`.
- `FindAlbum`: return nil on not found.

- [ ] **Step 4: Test**

Run:

```bash
gofmt -w internal/repository/music/music.go internal/repository/music/music_test.go
go test ./internal/repository/music -count=1
```

Expected: PASS.

- [ ] **Step 5: Stop and report**

Report changed files and tests.

---

## Task 5: Repository Write Operations

**Files:**
- Modify: `internal/repository/music/music.go`
- Modify: `internal/repository/music/music_test.go`

- [ ] **Step 1: Add write tests**

Add:

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

- [ ] **Step 2: Verify tests fail**

Run:

```bash
go test ./internal/repository/music -run 'SaveMusic' -count=1
```

Expected: FAIL because write methods do not exist.

- [ ] **Step 3: Extend repository contract**

Add:

```go
type MusicSaveData struct {
	Music           model.Music
	ArtistRelations []model.MusicArtistRelation
}
```

Add interface methods:

```go
SaveMusic(data MusicSaveData) error
DeleteMusic(id uint) error
SaveArtist(artist model.MusicArtist) (*model.MusicArtist, error)
DeleteArtist(id uint) error
SaveAlbum(album model.MusicAlbum) (*model.MusicAlbum, error)
DeleteAlbum(id uint) error
```

Implementation rules:

- `SaveMusic` uses transaction.
- For new music (`ID == 0`), create first, then relation rows with the allocated ID.
- For existing music, verify it exists, update fields, delete old relations, create new relations.
- `DeleteMusic`, `DeleteArtist`, `DeleteAlbum` use GORM soft delete and return nil if row does not exist.
- `SaveArtist` and `SaveAlbum` create on ID zero; update known fields on nonzero ID.

- [ ] **Step 4: Test**

Run:

```bash
gofmt -w internal/repository/music/music.go internal/repository/music/music_test.go
go test ./internal/repository/music -count=1
```

Expected: PASS.

- [ ] **Step 5: Stop and report**

Report changed files and tests.

---

## Task 6: Service Mapping And Read APIs

**Files:**
- Modify: `internal/service/music/music.go`
- Modify: `internal/service/music/music_test.go`

- [ ] **Step 1: Update service test stubs**

Expand the stub repository in `internal/service/music/music_test.go` so it implements the new repository interface. For methods not used by a test, return zero values.

Use concrete method signatures from Tasks 4 and 5. Do not use `any` or `interface{}` in service/repository signatures.

- [ ] **Step 2: Add service read tests**

Add:

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
```

- [ ] **Step 3: Verify tests fail**

Run:

```bash
go test ./internal/service/music -run 'ListPublic' -count=1
```

Expected: FAIL because `ListPublic` does not exist.

- [ ] **Step 4: Implement read service methods**

Modify `MusicService` interface:

```go
type MusicService interface {
	List() (*dto.MusicListResp, error)
	ListPublic() (*dto.MusicListResp, error)
	GetPublicDetail(id uint) (*dto.MusicDetailResp, error)
	ListArtists(keyword string) (*dto.MusicArtistListResp, error)
	ListAlbums(keyword string) (*dto.MusicAlbumListResp, error)
}
```

Keep `List()` as:

```go
func (s *musicService) List() (*dto.MusicListResp, error) {
	return s.ListPublic()
}
```

Mapping rules:

- Resolve `AudioKey` into `AudioURL`.
- For album cover, resolve `MusicAlbum.CoverKey` into `CoverURL`.
- Artist `DisplayName` uses `ArtistDisplayName(artist.Name, artist.NameZh)`.
- `GetPublicDetail` returns `ErrMusicNotFound` if repo returns nil.

Add errors:

```go
var (
	ErrMusicNotFound       = errors.New("音乐不存在")
	ErrMusicArtistNotFound = errors.New("歌手不存在")
	ErrMusicAlbumNotFound  = errors.New("专辑不存在")
)
```

- [ ] **Step 5: Test**

Run:

```bash
gofmt -w internal/service/music/music.go internal/service/music/music_test.go
go test ./internal/service/music -count=1
```

Expected: PASS.

- [ ] **Step 6: Stop and report**

Report changed files and tests.

---

## Task 7: Service Write APIs

**Files:**
- Modify: `internal/service/music/music.go`
- Modify: `internal/service/music/music_test.go`

- [ ] **Step 1: Add save validation tests**

Add:

```go
func TestMusicService_SaveMusic_RejectsMissingArtists(t *testing.T) {
	repo := &stubMusicRepository{
		artists: []model.MusicArtist{{Base: model.Base{ID: 1}, Name: "Aimer"}},
	}
	svc := music.NewMusicService(repo, nil)

	err := svc.SaveMusic(dto.MusicSaveReq{
		Name: "Song",
		ArtistIDs: []uint{1, 2},
		AudioKey: "music/audio/1/a.mp3",
		IsPublic: true,
	})

	require.ErrorIs(t, err, music.ErrMusicArtistNotFound)
}
```

- [ ] **Step 2: Verify tests fail**

Run:

```bash
go test ./internal/service/music -run 'SaveMusic' -count=1
```

Expected: FAIL because write service methods do not exist.

- [ ] **Step 3: Extend service interface**

Add:

```go
SaveMusic(req dto.MusicSaveReq) error
DeleteMusic(id uint) error
SaveArtist(req dto.MusicArtistSaveReq) (*dto.MusicArtistResp, error)
DeleteArtist(id uint) error
SaveAlbum(req dto.MusicAlbumSaveReq) (*dto.MusicAlbumResp, error)
DeleteAlbum(id uint) error
```

Implementation rules:

- `SaveMusic` loads all artist IDs; if count mismatches request unique IDs, return `ErrMusicArtistNotFound`.
- If `AlbumID` is nonnil, load album; nil result returns `ErrMusicAlbumNotFound`.
- If `ArtistDisplayName` is blank, join selected artist display names with ` / `.
- Build `model.MusicArtistRelation` rows in request order, role `primary`.
- Save `AudioKey` into both `Music.AudioKey` and legacy `Music.URL` for first-phase compatibility.
- `SaveArtist` returns DTO with formatted `DisplayName`.
- `SaveAlbum` validates `ArtistID` if present.

- [ ] **Step 4: Test**

Run:

```bash
gofmt -w internal/service/music/music.go internal/service/music/music_test.go
go test ./internal/service/music -count=1
```

Expected: PASS.

- [ ] **Step 5: Stop and report**

Report changed files and tests.

---

## Task 8: Public Music Handlers And Route Order

**Files:**
- Modify: `internal/handler/music/music.go`
- Modify: `internal/handler/music/music_test.go`
- Modify: `internal/router/router.go`

- [ ] **Step 1: Add handler tests**

Update the stub service in `internal/handler/music/music_test.go` for new interface methods. Add:

```go
func TestMusicHandler_GetPublicDetail_Success(t *testing.T) {
	svc := &stubMusicService{detail: &dto.MusicDetailResp{MusicItemResp: dto.MusicItemResp{ID: 1, Name: "Song"}}}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := music.NewMusicHandler(svc)
	r.GET("/music/:id", h.GetPublicDetail)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/music/1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
```

- [ ] **Step 2: Verify tests fail**

Run:

```bash
go test ./internal/handler/music -run 'GetPublicDetail' -count=1
```

Expected: FAIL because method does not exist.

- [ ] **Step 3: Implement public handlers**

Add methods:

- `List`
- `GetPublicDetail`
- `ListArtists`
- `ListAlbums`

Rules:

- Parse `:id` with `strconv.ParseUint`.
- Bad ID returns `response.Fail(c, response.CodeBadRequest, "参数错误")`.
- `ErrMusicNotFound` returns `response.NotFound(c)`.
- Use `response.Success`, never `c.JSON`.
- Add Swagger annotations above each handler.

- [ ] **Step 4: Register routes with safe order**

In `registerPublicRoutes`, static routes must be before `/:id`:

```go
r.GET("/music", handlers.music.List)
r.GET("/music/artists", handlers.music.ListArtists)
r.GET("/music/albums", handlers.music.ListAlbums)
r.GET("/music/:id", handlers.music.GetPublicDetail)
```

Do not register `/music/:id` before `/music/artists` or `/music/albums`.

- [ ] **Step 5: Test**

Run:

```bash
gofmt -w internal/handler/music/music.go internal/handler/music/music_test.go internal/router/router.go
go test ./internal/handler/music ./internal/router -count=1
```

Expected: PASS.

- [ ] **Step 6: Stop and report**

Report changed files and tests.

---

## Task 9: Admin Artist And Album Handlers

**Files:**
- Modify: `internal/handler/music/music.go`
- Modify: `internal/handler/music/music_test.go`
- Modify: `internal/router/router.go`

- [ ] **Step 1: Add tests**

Add tests for bad JSON and successful save:

```go
func TestMusicHandler_SaveArtist_BadJSON(t *testing.T) {
	svc := &stubMusicService{}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := music.NewMusicHandler(svc)
	r.POST("/admin/music/artists", h.SaveArtist)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/music/artists", strings.NewReader("{"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
```

- [ ] **Step 2: Implement handlers**

Add:

- `ListAdminArtists`
- `SaveArtist`
- `DeleteArtist`
- `ListAdminAlbums`
- `SaveAlbum`
- `DeleteAlbum`

Rules:

- Bind JSON for save.
- Bind query `keyword` for list.
- For `PUT`, read ID from path and assign it to `dto.MusicArtistSaveReq.ID` or `dto.MusicAlbumSaveReq.ID` before calling service.
- Delete parses `:id`, bad ID is business bad request.
- Map service not-found errors to `response.NotFound(c)`.

- [ ] **Step 3: Register admin routes**

Inside `registerAdminRoutes`:

```go
admin.GET("/music/artists", handlers.music.ListAdminArtists)
admin.POST("/music/artists", handlers.music.SaveArtist)
admin.PUT("/music/artists/:id", handlers.music.SaveArtist)
admin.DELETE("/music/artists/:id", handlers.music.DeleteArtist)
admin.GET("/music/albums", handlers.music.ListAdminAlbums)
admin.POST("/music/albums", handlers.music.SaveAlbum)
admin.PUT("/music/albums/:id", handlers.music.SaveAlbum)
admin.DELETE("/music/albums/:id", handlers.music.DeleteAlbum)
```

- [ ] **Step 4: Test**

Run:

```bash
gofmt -w internal/handler/music/music.go internal/handler/music/music_test.go internal/router/router.go internal/dto/music.go
go test ./internal/handler/music ./internal/router -count=1
```

Expected: PASS.

- [ ] **Step 5: Stop and report**

Report changed files and tests.

---

## Task 10: Admin Music Handlers

**Files:**
- Modify: `internal/handler/music/music.go`
- Modify: `internal/handler/music/music_test.go`
- Modify: `internal/router/router.go`
- Modify: `internal/dto/music.go`

- [ ] **Step 1: Use request ID field**

`dto.MusicSaveReq` already has `ID uint`. Handler must set `req.ID` from the path on `PUT /admin/music/:id` before calling service.

- [ ] **Step 2: Add tests**

Add:

```go
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

- [ ] **Step 3: Implement handlers**

Add:

- `ListAdmin`
- `SaveMusic`
- `DeleteMusic`

Rules:

- `ListAdmin` binds `dto.MusicAdminListReq`.
- Default `Page=1`, `PageSize=20` in service or handler.
- `PageSize` max is 100.
- `SaveMusic` maps missing artist/album service errors to `response.Fail(c, response.CodeBadRequest, err.Error())`.
- `DeleteMusic` returns success with deleted ID or empty object.

- [ ] **Step 4: Register routes**

Inside admin group:

```go
admin.GET("/music", handlers.music.ListAdmin)
admin.POST("/music", handlers.music.SaveMusic)
admin.PUT("/music/:id", handlers.music.SaveMusic)
admin.DELETE("/music/:id", handlers.music.DeleteMusic)
```

Place `/admin/music/artists` and `/admin/music/albums` routes before `/admin/music/:id` if the router ever groups them manually. Gin route tree usually accepts both, but keep static routes first for clarity.

- [ ] **Step 5: Test**

Run:

```bash
gofmt -w internal/handler/music/music.go internal/handler/music/music_test.go internal/router/router.go internal/dto/music.go
go test ./internal/handler/music ./internal/router -count=1
```

Expected: PASS.

- [ ] **Step 6: Stop and report**

Report changed files and tests.

---

## Task 11: Minimal Audio Validator

**Files:**
- Create: `pkg/audiofile/audiofile.go`
- Create: `pkg/audiofile/audiofile_test.go`

- [ ] **Step 1: Create tests**

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
	assert.Equal(t, uint64(len(data)), result.Size)
}

func TestValidateRejectsInvalidAudio(t *testing.T) {
	_, err := audiofile.Validate("song.txt", []byte("not audio"), 1024)

	require.ErrorIs(t, err, audiofile.ErrInvalidAudio)
}

func TestValidateRejectsTooLarge(t *testing.T) {
	_, err := audiofile.Validate("song.mp3", append([]byte("ID3"), make([]byte, 20)...), 4)

	require.ErrorIs(t, err, audiofile.ErrAudioTooLarge)
}
```

- [ ] **Step 2: Implement validator**

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

- [ ] **Step 3: Test**

Run:

```bash
gofmt -w pkg/audiofile/audiofile.go pkg/audiofile/audiofile_test.go
go test ./pkg/audiofile -count=1
```

Expected: PASS.

- [ ] **Step 4: Stop and report**

Report changed files and tests.

---

## Task 12: Music Upload Service And Handlers

**Files:**
- Create: `internal/service/music/upload.go`
- Modify: `internal/service/music/music.go`
- Modify: `internal/service/music/music_test.go`
- Modify: `internal/handler/music/music.go`
- Modify: `internal/handler/music/music_test.go`
- Modify: `internal/router/router.go`

- [ ] **Step 1: Service contract**

Extend `MusicService`:

```go
UploadAudio(ctx context.Context, input MusicAudioUploadInput) (*dto.MusicUploadResp, error)
UploadAlbumCover(ctx context.Context, input MusicImageUploadInput) (*dto.MusicUploadResp, error)
UploadArtistAvatar(ctx context.Context, input MusicImageUploadInput) (*dto.MusicUploadResp, error)
```

Define input types in `internal/service/music/upload.go`:

```go
type MusicAudioUploadInput struct {
	UserID uint
	Name   string
	Data   []byte
}

type MusicImageUploadInput struct {
	UserID uint
	Name   string
	Data   []byte
}
```

- [ ] **Step 2: Implement service**

Implementation requirements:

- Audio max size: `50 * 1024 * 1024`.
- Image max size: reuse `uploadservice.MaxTempImageBytes` or `10 * 1024 * 1024`.
- Audio key: `temp/music/{user_id}/audio/{sha256}{ext}`.
- Album cover key: `temp/music/{user_id}/album-cover/{md5}{ext}`.
- Artist avatar key: `temp/music/{user_id}/artist-avatar/{md5}{ext}`.
- Check `ObjectExists`; skip `PutObject` on duplicate.
- Return URL via `ObjectURL`.
- Map invalid audio/image to `ErrMusicUploadInvalid`.

Add:

```go
var ErrMusicUploadInvalid = errors.New("音乐资源无效")
```

- [ ] **Step 3: Implement handlers**

Add:

- `UploadAudio`
- `UploadAlbumCover`
- `UploadArtistAvatar`

Rules:

- Use `jwt.GetClaims(c)` and require logged-in admin route context.
- Read multipart file field named `file`.
- Use `io.LimitReader` with max + 1.
- Oversized file returns `response.Fail(c, response.CodeBadRequest, "文件过大")`.
- Invalid upload returns `response.Fail(c, response.CodeBadRequest, err.Error())`.

- [ ] **Step 4: Register routes**

Inside admin group:

```go
admin.POST("/music/uploads/audio", handlers.music.UploadAudio)
admin.POST("/music/uploads/album-cover", handlers.music.UploadAlbumCover)
admin.POST("/music/uploads/artist-avatar", handlers.music.UploadArtistAvatar)
```

- [ ] **Step 5: Test**

Run:

```bash
gofmt -w internal/service/music internal/handler/music internal/router/router.go
go test ./pkg/audiofile ./internal/service/music ./internal/handler/music ./internal/router -run 'Upload|Validate|Music' -count=1
```

Expected: PASS.

- [ ] **Step 6: Stop and report**

Report changed files and tests.

---

## Task 13: Article Music ID Validation

**Files:**
- Modify: `internal/repository/article/article.go`
- Modify: `internal/repository/article/mutation.go`
- Modify: `internal/repository/article/article_test.go`
- Modify: `internal/service/article/article.go`
- Modify: `internal/service/article/article_test.go`

- [ ] **Step 1: Inspect existing article save flow**

Read:

```bash
sed -n '1,120p' internal/repository/article/article.go
sed -n '1,90p' internal/repository/article/mutation.go
sed -n '150,230p' internal/service/article/article.go
```

- [ ] **Step 2: Add repository existence check**

Add this method to `ArticleRepository` in `internal/repository/article/article.go`:

```go
CountExistingMusicIDs(ids []uint) (int64, error)
```

Implement it on `articleRepo` in `internal/repository/article/query.go`:

```go
func (r *articleRepo) CountExistingMusicIDs(ids []uint) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var count int64
	err := r.db.Model(&model.Music{}).Where("id IN ?", ids).Count(&count).Error
	return count, err
}
```

- [ ] **Step 3: Validate in service before save**

In article service save path, after `uniqueUintIDs(req.MusicIDs)`, check count equals number of unique music IDs. If not, return a business error like:

```go
var ErrArticleMusicNotFound = errors.New("音乐不存在")
```

Map this error in `internal/handler/article/article.go` to `response.Fail(c, response.CodeBadRequest, "音乐不存在")`.

- [ ] **Step 4: Tests**

Add service test for missing music ID. Use existing article service fake repository; extend fake with `CountExistingMusicIDs`.

Run:

```bash
gofmt -w internal/repository/article internal/service/article
go test ./internal/repository/article ./internal/service/article ./internal/handler/article -run 'Music|Save|Article' -count=1
```

Expected: PASS.

- [ ] **Step 5: Stop and report**

Report changed files and tests.

---

## Task 14: Old Migration Tool

**Files:**
- Modify: `cmd/migrate/main.go`
- Modify: `cmd/migrate/main_test.go`

- [ ] **Step 1: Add helper tests**

In `cmd/migrate/main_test.go`, add:

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

	require.Len(t, seeds, 3)
	assert.Equal(t, "Aimer", seeds[0].Name)
	assert.Equal(t, "milet", seeds[1].Name)
	assert.Equal(t, "幾田りら", seeds[2].Name)
}
```

- [ ] **Step 2: Implement helper**

Add helper in `cmd/migrate/main.go`:

```go
type musicArtistSeed struct {
	Name   string
	NameZh *string
}

func buildMusicArtistSeeds(value string) []musicArtistSeed {
	tokens := musicservice.SplitArtistTokens(value)
	if len(tokens) == 0 && strings.TrimSpace(value) != "" {
		tokens = []string{strings.TrimSpace(value)}
	}
	seeds := make([]musicArtistSeed, 0, len(tokens))
	for _, token := range tokens {
		name, nameZh := musicservice.SplitArtistDisplayName(token)
		if name == "" {
			continue
		}
		seeds = append(seeds, musicArtistSeed{Name: name, NameZh: nameZh})
	}
	return seeds
}
```

Import `musicservice "github.com/vpt/blog-backend/internal/service/music"`.

- [ ] **Step 3: Register models**

In `autoMigrate`, add before `&model.Music{}`:

```go
&model.MusicArtist{},
&model.MusicAlbum{},
&model.MusicArtistRelation{},
```

- [ ] **Step 4: Update `migrateMusic`**

Modify `migrateMusic` so it:

- Creates/fetches artists from `singer`.
- Creates/fetches album from `album` with first artist as `artist_id` when available.
- Creates `model.Music` with original ID preserved.
- Sets `ArtistDisplayName` to old `singer`.
- Sets `AudioKey` to old `url`.
- Keeps legacy `URL`, `CoverImgUrl`, `Singer`, `Album`.
- Writes `music_artist_relation` rows after music create.

Do not move Garage objects in this task.

- [ ] **Step 5: Test**

Run:

```bash
gofmt -w cmd/migrate/main.go cmd/migrate/main_test.go
go test ./cmd/migrate -run 'TestBuildMusicArtistSeeds|TestBuildMomentMediaGaragePlan|TestBuildArticleGaragePlan' -count=1
```

Expected: PASS.

- [ ] **Step 6: Stop and report**

Report changed files and tests.

---

## Task 15: Garage Migration Planning For Music Assets

**Files:**
- Modify: `cmd/migrate/main.go`
- Modify: `cmd/migrate/main_test.go`

- [ ] **Step 1: Add plan struct**

Add:

```go
type musicGaragePlan struct {
	MusicID uint
	AlbumID *uint
	SourceAudioKey string
	TargetAudioKey string
	SourceCoverKey string
	TargetCoverKey string
}
```

- [ ] **Step 2: Add pure planning tests**

Test only key planning, not real Garage:

```go
func TestBuildMusicGaragePlan_RewritesAudioAndCover(t *testing.T) {
	albumID := uint(8)
	plan := buildMusicGaragePlan(3, &albumID, "old/song.mp3", "old/cover.jpg")

	assert.Equal(t, "old/song.mp3", plan.SourceAudioKey)
	assert.Contains(t, plan.TargetAudioKey, "music/audio/3/")
	assert.Equal(t, "old/cover.jpg", plan.SourceCoverKey)
	assert.Contains(t, plan.TargetCoverKey, "music/albums/8/cover/")
}
```

- [ ] **Step 3: Implement planning**

Rules:

- For old audio key `x/y/song.mp3`, target `music/audio/{music_id}/song.mp3` if no hash is available.
- For old cover key `x/y/cover.jpg`, target `music/albums/{album_id}/cover/cover.jpg`.
- If value is absolute external URL and `ObjectKey` cannot parse it, skip planning and leave fields unchanged.

- [ ] **Step 4: Hook into `migrateGarageObjects`**

Add a music asset migration phase after article and moment media migration. It must:

- Query migrated `music.id`, `music.audio_key`, `music_album.id`, `music_album.cover_key`.
- Copy source to target with existing `copyObjectIfNeeded`.
- Update DB only after copy success.
- Log skipped external URLs.
- Never delete source keys.

- [ ] **Step 5: Test**

Run:

```bash
gofmt -w cmd/migrate/main.go cmd/migrate/main_test.go
go test ./cmd/migrate -run 'TestBuildMusicGaragePlan|TestBuildMomentMediaGaragePlan|TestBuildArticleGaragePlan' -count=1
```

Expected: PASS.

- [ ] **Step 6: Stop and report**

Report changed files and tests.

---

## Task 16: Swagger And Final Verification

**Files:**
- Modify generated Swagger files under `docs/`

- [ ] **Step 1: Generate Swagger**

Run:

```bash
make swag
```

Expected: PASS. Swagger includes:

```text
/music
/music/{id}
/music/artists
/music/albums
/admin/music
/admin/music/artists
/admin/music/albums
/admin/music/uploads/audio
/admin/music/uploads/album-cover
/admin/music/uploads/artist-avatar
```

- [ ] **Step 2: Run targeted tests**

Run:

```bash
go test ./pkg/audiofile ./pkg/storage ./internal/repository/music ./internal/service/music ./internal/handler/music ./internal/router ./cmd/migrate -count=1
```

Expected: PASS.

- [ ] **Step 3: Run article tests**

Run:

```bash
go test ./internal/repository/article ./internal/service/article ./internal/handler/article -count=1
```

Expected: PASS.

- [ ] **Step 4: Inspect final diff**

Run:

```bash
git diff --stat
git diff -- internal/model/music.go internal/router/router.go migrations/20260625_music_catalog.sql
```

Expected: only music catalog, migration, upload, article music validation, migration tool, tests, and Swagger files changed.

- [ ] **Step 5: Stop and report**

Report:

```text
Final verification complete.
Commands run:
- make swag
- go test ./pkg/audiofile ./pkg/storage ./internal/repository/music ./internal/service/music ./internal/handler/music ./internal/router ./cmd/migrate -count=1
- go test ./internal/repository/article ./internal/service/article ./internal/handler/article -count=1
Known risks:
- First phase does not parse ID3 metadata.
- Old Garage keys are copied, not deleted.
```

---

## Cursor Auto Prompt

Use this prompt when assigning one task to Cursor Auto. Replace `{TASK_NUMBER}` before sending.

```text
You are working in /Volumes/External/SynologyDrive/Codes/Blog/blog-backend.

Read AGENTS.md first. Then read:
- docs/superpowers/specs/2026-06-25-music-catalog-design.md
- docs/superpowers/plans/2026-06-25-music-catalog.md

Implement ONLY Task {TASK_NUMBER} from docs/superpowers/plans/2026-06-25-music-catalog.md.

Hard constraints:
- Only edit files listed in Task {TASK_NUMBER}.
- Do not implement later tasks.
- Do not delete legacy music columns or fields.
- Do not expose model.* in DTOs or Swagger.
- Do not introduce package-level globals for db, storage, logger, or services.
- Follow the existing handler/service/repository layering.
- Use response.Success/Fail/NotFound/etc. in handlers, never c.JSON directly.
- Run the exact test command listed in the task.
- Stop after this task. Do not continue to the next task.

When done, report:
1. Files changed
2. Tests run and results
3. Any failures or unclear points
4. Whether the task is complete
```

## Self-Review

- Spec coverage: artist, album, song, multi-artist relation, Chinese translated names, Garage paths, uploads, migration, and article music validation each have a task.
- Cheap-agent hardening: tasks are smaller than the original plan, each has exact file boundaries, explicit non-goals, and stop points.
- Known implementation choice: first phase uploads validate audio bytes and hash them, but does not parse ID3 metadata.
