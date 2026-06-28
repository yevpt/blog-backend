package guestbook

import (
	"context"
	"math"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	guestbookrepo "github.com/vpt/blog-backend/internal/repository/guestbook"
	"github.com/vpt/blog-backend/internal/service/commentasset"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/internal/service/userrole"
	"github.com/vpt/blog-backend/pkg/storage"
)

func guestbookPageToDTO(result *guestbookrepo.PageResult, resolver storage.ObjectURLResolver, rolesMap map[uint][]string, views map[moderationservice.SubjectKey]moderationservice.View) *dto.GuestbookPageResp {
	items := make([]dto.GuestbookItemResp, 0, len(result.Messages))
	for _, message := range result.Messages {
		items = append(items, *guestbookItemToDTO(message, resolver, rolesMap, views))
	}

	pages := 0
	if result.PageSize > 0 {
		pages = int(math.Ceil(float64(result.Total) / float64(result.PageSize)))
	}

	return &dto.GuestbookPageResp{
		Total:    result.Total,
		Pages:    pages,
		Page:     result.Page,
		PageSize: result.PageSize,
		List:     items,
	}
}

func adminGuestbookPageToDTO(result *guestbookrepo.PageResult, resolver storage.ObjectURLResolver, rolesMap map[uint][]string, views map[moderationservice.SubjectKey]moderationservice.View) *dto.AdminGuestbookPageResp {
	items := make([]dto.GuestbookItemResp, 0, len(result.Messages))
	for _, message := range result.Messages {
		items = append(items, *guestbookItemToDTO(message, resolver, rolesMap, views))
	}

	pages := 0
	if result.PageSize > 0 {
		pages = int(math.Ceil(float64(result.Total) / float64(result.PageSize)))
	}

	return &dto.AdminGuestbookPageResp{
		Total:    result.Total,
		Pages:    pages,
		Page:     result.Page,
		PageSize: result.PageSize,
		List:     items,
	}
}

func guestbookItemToDTO(aggregate guestbookrepo.GuestbookAggregate, resolver storage.ObjectURLResolver, rolesMap map[uint][]string, views map[moderationservice.SubjectKey]moderationservice.View) *dto.GuestbookItemResp {
	message := aggregate.Message
	result := &dto.GuestbookItemResp{
		ID:          message.ID,
		OwnerUserID: message.OwnerUserID,
		FromUserID:  message.FromUserID,
		Content:     commentasset.ResolveContent(context.Background(), resolver, message.Content),
		User:        guestbookUserToDTO(aggregate.User, resolver, rolesMap),
		ReplyCount:  aggregate.ReplyCount,
		LikeCount:   aggregate.LikeCount,
		IsLiked:     aggregate.IsLiked,
		CreatedAt:   message.CreatedAt,
		UpdatedAt:   message.UpdatedAt,
	}
	view, ok := views[moderationservice.SubjectKey{ContentType: moderationservice.SubjectGuestbook, ContentID: uint64(message.ID)}]
	if ok {
		content, projected := moderationservice.ProjectView(view)
		result.Content = commentasset.ResolveContent(context.Background(), resolver, content)
		result.Moderation = projected
	}
	return result
}

func guestbookUserToDTO(user *model.User, resolver storage.ObjectURLResolver, rolesMap map[uint][]string) *dto.GuestbookUserResp {
	if user == nil {
		return nil
	}
	return &dto.GuestbookUserResp{
		ID:        user.ID,
		Username:  user.Username,
		Nickname:  user.Nickname,
		AvatarUrl: storage.ResolvePtrURL(resolver, user.AvatarUrl),
		Site:      user.Site,
		Mark:      user.Mark,
		Roles:     userrole.ForUser(rolesMap, user.ID),
	}
}

func collectGuestbookPageUserIDs(result *guestbookrepo.PageResult) []uint {
	if result == nil {
		return nil
	}
	ids := make([]uint, 0, len(result.Messages))
	for _, aggregate := range result.Messages {
		if aggregate.User != nil {
			ids = append(ids, aggregate.User.ID)
		}
	}
	return ids
}

func collectGuestbookAggregateUserIDs(aggregate *guestbookrepo.GuestbookAggregate) []uint {
	if aggregate == nil || aggregate.User == nil {
		return nil
	}
	return []uint{aggregate.User.ID}
}
