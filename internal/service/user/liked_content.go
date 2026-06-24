package user

import (
	"math"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	userrepo "github.com/vpt/blog-backend/internal/repository/user"
	"github.com/vpt/blog-backend/pkg/storage"
)

const defaultLikedContentPageSize = 20
const maxLikedContentPageSize = 50

// ListLikedContent 分页返回用户赞过的公开内容。
func (s *userService) ListLikedContent(userID uint, req dto.UserLikedContentListReq) (*dto.UserLikedContentPageResp, error) {
	page := normalizeLikedContentPage(req.Page)
	pageSize := normalizeLikedContentPageSize(req.PageSize)
	filter := userrepo.LikedContentFilter{
		UserID:   userID,
		Type:     normalizeLikedContentType(req.Type),
		Page:     page,
		PageSize: pageSize,
	}

	result, err := s.repo.ListLikedContent(filter)
	if err != nil {
		return nil, err
	}
	return s.likedContentPageToDTO(result)
}

func normalizeLikedContentPage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}

func normalizeLikedContentPageSize(pageSize int) int {
	if pageSize < 1 {
		return defaultLikedContentPageSize
	}
	if pageSize > maxLikedContentPageSize {
		return defaultLikedContentPageSize
	}
	return pageSize
}

func normalizeLikedContentType(value string) string {
	switch value {
	case dto.UserLikedContentFilterArticle,
		dto.UserLikedContentFilterComment,
		dto.UserLikedContentFilterGuestbook,
		dto.UserLikedContentFilterMoment:
		return value
	default:
		return ""
	}
}

func (s *userService) likedContentPageToDTO(result *userrepo.LikedContentPageResult) (*dto.UserLikedContentPageResp, error) {
	if result == nil {
		return &dto.UserLikedContentPageResp{List: []dto.UserLikedContentItemResp{}}, nil
	}

	roles, err := s.likedContentAuthorRoles(result.Items)
	if err != nil {
		return nil, err
	}
	items := make([]dto.UserLikedContentItemResp, 0, len(result.Items))
	for _, aggregate := range result.Items {
		items = append(items, likedContentItemToDTO(aggregate, roles, s.resolver))
	}

	pages := 0
	if result.Total > 0 && result.PageSize > 0 {
		pages = int(math.Ceil(float64(result.Total) / float64(result.PageSize)))
	}
	return &dto.UserLikedContentPageResp{
		Total:    result.Total,
		Pages:    pages,
		Page:     result.Page,
		PageSize: result.PageSize,
		List:     items,
	}, nil
}

func (s *userService) likedContentAuthorRoles(items []userrepo.LikedContentAggregate) (map[uint][]string, error) {
	ids := make([]uint, 0, len(items))
	seen := make(map[uint]struct{}, len(items))
	for _, item := range items {
		if item.Author == nil || item.Author.ID == 0 {
			continue
		}
		if _, ok := seen[item.Author.ID]; ok {
			continue
		}
		seen[item.Author.ID] = struct{}{}
		ids = append(ids, item.Author.ID)
	}
	if len(ids) == 0 {
		return map[uint][]string{}, nil
	}
	return s.repo.FindRolesByUserIDs(ids)
}

func likedContentItemToDTO(
	aggregate userrepo.LikedContentAggregate,
	roles map[uint][]string,
	resolver storage.ObjectURLResolver,
) dto.UserLikedContentItemResp {
	return dto.UserLikedContentItemResp{
		ID:      aggregate.ID,
		LikedAt: aggregate.LikedAt,
		Kind:    aggregate.Kind,
		Filter:  aggregate.Filter,
		Author:  likedContentAuthorToDTO(aggregate.Author, roles, resolver),
		Content: likedContentObjectToDTO(aggregate.Content, resolver),
		Parent:  likedContentObjectPtrToDTO(aggregate.Parent, resolver),
		Root:    likedContentObjectPtrToDTO(aggregate.Root, resolver),
		ToUser:  likedContentAuthorToDTO(aggregate.ToUser, nil, resolver),
		Stats:   likedContentStatsToDTO(aggregate.Stats),
	}
}

func likedContentAuthorToDTO(
	author *model.User,
	roles map[uint][]string,
	resolver storage.ObjectURLResolver,
) *dto.UserLikedContentAuthorResp {
	if author == nil {
		return nil
	}
	return &dto.UserLikedContentAuthorResp{
		ID:        author.ID,
		Username:  author.Username,
		Nickname:  author.Nickname,
		AvatarUrl: storage.ResolvePtrURL(resolver, author.AvatarUrl),
		Site:      author.Site,
		Mark:      author.Mark,
		Roles:     append([]string(nil), roles[author.ID]...),
	}
}

func likedContentObjectToDTO(object userrepo.LikedContentObject, resolver storage.ObjectURLResolver) dto.UserLikedContentObjectResp {
	return dto.UserLikedContentObjectResp{
		ID:          object.ID,
		Kind:        object.Kind,
		Title:       object.Title,
		Excerpt:     object.Excerpt,
		CoverImgUrl: storage.ResolvePtrURL(resolver, object.CoverImgURL),
		Deleted:     object.Deleted,
	}
}

func likedContentObjectPtrToDTO(object *userrepo.LikedContentObject, resolver storage.ObjectURLResolver) *dto.UserLikedContentObjectResp {
	if object == nil {
		return nil
	}
	dtoObject := likedContentObjectToDTO(*object, resolver)
	return &dtoObject
}

func likedContentStatsToDTO(stats *userrepo.LikedContentStats) *dto.UserLikedContentStatsResp {
	if stats == nil {
		return nil
	}
	return &dto.UserLikedContentStatsResp{
		LikeCount:    stats.LikeCount,
		CommentCount: stats.CommentCount,
	}
}
