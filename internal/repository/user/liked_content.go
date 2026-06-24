package user

import (
	"strings"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	articlerepo "github.com/vpt/blog-backend/internal/repository/article"
	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	guestbookrepo "github.com/vpt/blog-backend/internal/repository/guestbook"
	momentrepo "github.com/vpt/blog-backend/internal/repository/moment"
)

type likedContentSQLPart struct {
	query string
	args  []any
}

type likedContentRow struct {
	LikeID             uint      `gorm:"column:like_id"`
	LikedAt            time.Time `gorm:"column:liked_at"`
	Kind               string    `gorm:"column:kind"`
	Filter             string    `gorm:"column:filter_type"`
	ContentID          uint      `gorm:"column:content_id"`
	ContentTitle       *string   `gorm:"column:content_title"`
	ContentExcerpt     string    `gorm:"column:content_excerpt"`
	ContentCoverImgURL *string   `gorm:"column:content_cover_img_url"`
	ParentKind         *string   `gorm:"column:parent_kind"`
	ParentID           *uint     `gorm:"column:parent_id"`
	ParentExcerpt      *string   `gorm:"column:parent_excerpt"`
	RootKind           *string   `gorm:"column:root_kind"`
	RootID             *uint     `gorm:"column:root_id"`
	RootTitle          *string   `gorm:"column:root_title"`
	RootExcerpt        *string   `gorm:"column:root_excerpt"`
	AuthorID           *uint     `gorm:"column:author_id"`
	AuthorUsername     *string   `gorm:"column:author_username"`
	AuthorNickname     *string   `gorm:"column:author_nickname"`
	AuthorAvatarURL    *string   `gorm:"column:author_avatar_url"`
	AuthorSite         *string   `gorm:"column:author_site"`
	AuthorMark         *string   `gorm:"column:author_mark"`
}

// ListLikedContent 分页查询某个用户赞过的公开内容。
func (r *userRepo) ListLikedContent(filter LikedContentFilter) (*LikedContentPageResult, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	parts := likedContentSQLParts(filter.UserID, filter.Type)
	if len(parts) == 0 {
		return &LikedContentPageResult{Page: page, PageSize: pageSize, Items: []LikedContentAggregate{}}, nil
	}

	unionSQL, args := likedContentUnionSQL(parts)
	var total int64
	if err := r.db.Raw("SELECT count(*) FROM ("+unionSQL+") AS liked", args...).Scan(&total).Error; err != nil {
		return nil, err
	}

	rows := make([]likedContentRow, 0, pageSize)
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	query := "SELECT * FROM (" + unionSQL + ") AS liked ORDER BY liked_at DESC, like_id DESC LIMIT ? OFFSET ?"
	if err := r.db.Raw(query, queryArgs...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]LikedContentAggregate, 0, len(rows))
	for _, row := range rows {
		items = append(items, likedContentRowToAggregate(row))
	}
	return &LikedContentPageResult{
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Items:    items,
	}, nil
}

func likedContentUnionSQL(parts []likedContentSQLPart) (string, []any) {
	queries := make([]string, 0, len(parts))
	args := make([]any, 0, len(parts)*4)
	for _, part := range parts {
		queries = append(queries, part.query)
		args = append(args, part.args...)
	}
	return strings.Join(queries, " UNION ALL "), args
}

func likedContentSQLParts(userID uint, filterType string) []likedContentSQLPart {
	parts := make([]likedContentSQLPart, 0, 8)
	if filterType == "" || filterType == LikedContentFilterArticle {
		parts = append(parts, articleLikeSQLPart(userID))
	}
	if filterType == "" || filterType == LikedContentFilterComment {
		parts = append(parts,
			articleCommentLikeSQLPart(userID),
			articleReplyLikeSQLPart(userID),
			momentCommentLikeSQLPart(userID),
			momentReplyLikeSQLPart(userID),
			guestbookReplyLikeSQLPart(userID),
		)
	}
	if filterType == "" || filterType == LikedContentFilterGuestbook {
		parts = append(parts, guestbookLikeSQLPart(userID))
	}
	if filterType == "" || filterType == LikedContentFilterMoment {
		parts = append(parts, momentLikeSQLPart(userID))
	}
	return parts
}

func articleLikeSQLPart(userID uint) likedContentSQLPart {
	return likedContentSQLPart{
		query: likedContentBaseSelect(
			"? AS kind, ? AS filter_type",
			"a.id AS content_id, a.title AS content_title, COALESCE(NULLIF(a.short_content, ''), LEFT(a.content, 200), '') AS content_excerpt, a.cover_img_url AS content_cover_img_url",
			"NULL AS parent_kind, NULL AS parent_id, NULL AS parent_excerpt",
			"NULL AS root_kind, NULL AS root_id, NULL AS root_title, NULL AS root_excerpt",
			"author.id AS author_id, author.username AS author_username, author.nickname AS author_nickname, author.avatar_url AS author_avatar_url, author.site AS author_site, author.mark AS author_mark",
			"JOIN article a ON a.id = ul.target_id AND a.deleted_at IS NULL AND a.status IN (?, ?) LEFT JOIN user author ON author.id = a.user_id AND author.deleted_at IS NULL",
			"ul.user_id = ? AND ul.type = ? AND ul.deleted_at IS NULL",
		),
		args: []any{LikedContentKindArticle, LikedContentFilterArticle, model.ArticleStatusPublic, model.ArticleStatusEncrypted, userID, articlerepo.ArticleLikeType},
	}
}

func momentLikeSQLPart(userID uint) likedContentSQLPart {
	return likedContentSQLPart{
		query: likedContentBaseSelect(
			"? AS kind, ? AS filter_type",
			"m.id AS content_id, NULL AS content_title, m.content AS content_excerpt, NULL AS content_cover_img_url",
			"NULL AS parent_kind, NULL AS parent_id, NULL AS parent_excerpt",
			"NULL AS root_kind, NULL AS root_id, NULL AS root_title, NULL AS root_excerpt",
			"author.id AS author_id, author.username AS author_username, author.nickname AS author_nickname, author.avatar_url AS author_avatar_url, author.site AS author_site, author.mark AS author_mark",
			"JOIN moment m ON m.id = ul.target_id AND m.deleted_at IS NULL AND m.status = 1 LEFT JOIN user author ON author.id = m.user_id AND author.deleted_at IS NULL",
			"ul.user_id = ? AND ul.type = ? AND ul.deleted_at IS NULL",
		),
		args: []any{LikedContentKindMoment, LikedContentFilterMoment, userID, momentrepo.MomentLikeType},
	}
}

func guestbookLikeSQLPart(userID uint) likedContentSQLPart {
	return likedContentSQLPart{
		query: likedContentBaseSelect(
			"? AS kind, ? AS filter_type",
			"g.id AS content_id, NULL AS content_title, g.content AS content_excerpt, NULL AS content_cover_img_url",
			"NULL AS parent_kind, NULL AS parent_id, NULL AS parent_excerpt",
			"? AS root_kind, g.id AS root_id, NULL AS root_title, g.content AS root_excerpt",
			"author.id AS author_id, author.username AS author_username, author.nickname AS author_nickname, author.avatar_url AS author_avatar_url, author.site AS author_site, author.mark AS author_mark",
			"JOIN guestbook g ON g.id = ul.target_id AND g.deleted_at IS NULL LEFT JOIN user author ON author.id = g.from_user_id AND author.deleted_at IS NULL",
			"ul.user_id = ? AND ul.type = ? AND ul.deleted_at IS NULL",
		),
		args: []any{LikedContentKindGuestbook, LikedContentFilterGuestbook, LikedContentRootGuestbook, userID, guestbookrepo.LikeType},
	}
}

func articleCommentLikeSQLPart(userID uint) likedContentSQLPart {
	return likedContentSQLPart{
		query: likedContentBaseSelect(
			"? AS kind, ? AS filter_type",
			"c.id AS content_id, NULL AS content_title, c.content AS content_excerpt, NULL AS content_cover_img_url",
			"NULL AS parent_kind, NULL AS parent_id, NULL AS parent_excerpt",
			"? AS root_kind, a.id AS root_id, a.title AS root_title, COALESCE(NULLIF(a.short_content, ''), LEFT(a.content, 200), '') AS root_excerpt",
			"author.id AS author_id, author.username AS author_username, author.nickname AS author_nickname, author.avatar_url AS author_avatar_url, author.site AS author_site, author.mark AS author_mark",
			"JOIN article_comment c ON c.id = ul.target_id AND c.deleted_at IS NULL JOIN article a ON a.id = c.article_id AND a.deleted_at IS NULL AND a.status IN (?, ?) LEFT JOIN user author ON author.id = c.user_id AND author.deleted_at IS NULL",
			"ul.user_id = ? AND ul.type = ? AND ul.deleted_at IS NULL",
		),
		args: []any{LikedContentKindComment, LikedContentFilterComment, LikedContentRootArticle, model.ArticleStatusPublic, model.ArticleStatusEncrypted, userID, commentrepo.ArticleCommentLikeType},
	}
}

func articleReplyLikeSQLPart(userID uint) likedContentSQLPart {
	return likedContentSQLPart{
		query: likedContentBaseSelect(
			"? AS kind, ? AS filter_type",
			"r.id AS content_id, NULL AS content_title, r.content AS content_excerpt, NULL AS content_cover_img_url",
			"? AS parent_kind, c.id AS parent_id, c.content AS parent_excerpt",
			"? AS root_kind, a.id AS root_id, a.title AS root_title, COALESCE(NULLIF(a.short_content, ''), LEFT(a.content, 200), '') AS root_excerpt",
			"author.id AS author_id, author.username AS author_username, author.nickname AS author_nickname, author.avatar_url AS author_avatar_url, author.site AS author_site, author.mark AS author_mark",
			"JOIN article_comment_reply r ON r.id = ul.target_id AND r.deleted_at IS NULL JOIN article_comment c ON c.id = r.comment_id AND c.deleted_at IS NULL JOIN article a ON a.id = c.article_id AND a.deleted_at IS NULL AND a.status IN (?, ?) LEFT JOIN user author ON author.id = r.from_user_id AND author.deleted_at IS NULL",
			"ul.user_id = ? AND ul.type = ? AND ul.deleted_at IS NULL",
		),
		args: []any{LikedContentKindReply, LikedContentFilterComment, LikedContentKindComment, LikedContentRootArticle, model.ArticleStatusPublic, model.ArticleStatusEncrypted, userID, commentrepo.ArticleCommentReplyLikeType},
	}
}

func momentCommentLikeSQLPart(userID uint) likedContentSQLPart {
	return likedContentSQLPart{
		query: likedContentBaseSelect(
			"? AS kind, ? AS filter_type",
			"c.id AS content_id, NULL AS content_title, c.content AS content_excerpt, NULL AS content_cover_img_url",
			"NULL AS parent_kind, NULL AS parent_id, NULL AS parent_excerpt",
			"? AS root_kind, m.id AS root_id, NULL AS root_title, m.content AS root_excerpt",
			"author.id AS author_id, author.username AS author_username, author.nickname AS author_nickname, author.avatar_url AS author_avatar_url, author.site AS author_site, author.mark AS author_mark",
			"JOIN moment_comment c ON c.id = ul.target_id AND c.deleted_at IS NULL JOIN moment m ON m.id = c.moment_id AND m.deleted_at IS NULL AND m.status = 1 LEFT JOIN user author ON author.id = c.user_id AND author.deleted_at IS NULL",
			"ul.user_id = ? AND ul.type = ? AND ul.deleted_at IS NULL",
		),
		args: []any{LikedContentKindComment, LikedContentFilterComment, LikedContentRootMoment, userID, commentrepo.MomentCommentLikeType},
	}
}

func momentReplyLikeSQLPart(userID uint) likedContentSQLPart {
	return likedContentSQLPart{
		query: likedContentBaseSelect(
			"? AS kind, ? AS filter_type",
			"r.id AS content_id, NULL AS content_title, r.content AS content_excerpt, NULL AS content_cover_img_url",
			"? AS parent_kind, c.id AS parent_id, c.content AS parent_excerpt",
			"? AS root_kind, m.id AS root_id, NULL AS root_title, m.content AS root_excerpt",
			"author.id AS author_id, author.username AS author_username, author.nickname AS author_nickname, author.avatar_url AS author_avatar_url, author.site AS author_site, author.mark AS author_mark",
			"JOIN moment_comment_reply r ON r.id = ul.target_id AND r.deleted_at IS NULL JOIN moment_comment c ON c.id = r.comment_id AND c.deleted_at IS NULL JOIN moment m ON m.id = c.moment_id AND m.deleted_at IS NULL AND m.status = 1 LEFT JOIN user author ON author.id = r.from_user_id AND author.deleted_at IS NULL",
			"ul.user_id = ? AND ul.type = ? AND ul.deleted_at IS NULL",
		),
		args: []any{LikedContentKindReply, LikedContentFilterComment, LikedContentKindComment, LikedContentRootMoment, userID, commentrepo.MomentCommentReplyLikeType},
	}
}

func guestbookReplyLikeSQLPart(userID uint) likedContentSQLPart {
	return likedContentSQLPart{
		query: likedContentBaseSelect(
			"? AS kind, ? AS filter_type",
			"r.id AS content_id, NULL AS content_title, r.content AS content_excerpt, NULL AS content_cover_img_url",
			"? AS parent_kind, g.id AS parent_id, g.content AS parent_excerpt",
			"? AS root_kind, g.id AS root_id, NULL AS root_title, g.content AS root_excerpt",
			"author.id AS author_id, author.username AS author_username, author.nickname AS author_nickname, author.avatar_url AS author_avatar_url, author.site AS author_site, author.mark AS author_mark",
			"JOIN guestbook_reply r ON r.id = ul.target_id AND r.deleted_at IS NULL JOIN guestbook g ON g.id = r.comment_id AND g.deleted_at IS NULL LEFT JOIN user author ON author.id = r.from_user_id AND author.deleted_at IS NULL",
			"ul.user_id = ? AND ul.type = ? AND ul.deleted_at IS NULL",
		),
		args: []any{LikedContentKindReply, LikedContentFilterComment, LikedContentKindGuestbook, LikedContentRootGuestbook, userID, commentrepo.GuestbookReplyLikeType},
	}
}

func likedContentBaseSelect(kindExpr, contentExpr, parentExpr, rootExpr, authorExpr, joinExpr, whereExpr string) string {
	return "SELECT ul.id AS like_id, ul.created_at AS liked_at, " +
		kindExpr + ", " +
		contentExpr + ", " +
		parentExpr + ", " +
		rootExpr + ", " +
		authorExpr +
		" FROM user_like ul " + joinExpr +
		" WHERE " + whereExpr
}

func likedContentRowToAggregate(row likedContentRow) LikedContentAggregate {
	return LikedContentAggregate{
		ID:      row.LikeID,
		LikedAt: row.LikedAt,
		Kind:    row.Kind,
		Filter:  row.Filter,
		Author:  likedContentAuthorFromRow(row),
		Content: LikedContentObject{
			ID:          row.ContentID,
			Kind:        row.Kind,
			Title:       row.ContentTitle,
			Excerpt:     row.ContentExcerpt,
			CoverImgURL: row.ContentCoverImgURL,
		},
		Parent: likedContentParentFromRow(row),
		Root:   likedContentRootFromRow(row),
	}
}

func likedContentAuthorFromRow(row likedContentRow) *model.User {
	if row.AuthorID == nil {
		return nil
	}
	user := &model.User{
		Base:      model.Base{ID: *row.AuthorID},
		Nickname:  row.AuthorNickname,
		AvatarUrl: row.AuthorAvatarURL,
		Site:      row.AuthorSite,
		Mark:      row.AuthorMark,
	}
	if row.AuthorUsername != nil {
		user.Username = *row.AuthorUsername
	}
	return user
}

func likedContentParentFromRow(row likedContentRow) *LikedContentObject {
	if row.ParentID == nil || row.ParentKind == nil {
		return nil
	}
	return &LikedContentObject{
		ID:      *row.ParentID,
		Kind:    *row.ParentKind,
		Excerpt: stringValue(row.ParentExcerpt),
	}
}

func likedContentRootFromRow(row likedContentRow) *LikedContentObject {
	if row.RootID == nil || row.RootKind == nil {
		return nil
	}
	return &LikedContentObject{
		ID:      *row.RootID,
		Kind:    *row.RootKind,
		Title:   row.RootTitle,
		Excerpt: stringValue(row.RootExcerpt),
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
