package guestbook

import (
	"math"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	guestbookrepo "github.com/vpt/blog-backend/internal/repository/guestbook"
	"github.com/vpt/blog-backend/pkg/storage"
)

func guestbookPageToDTO(result *guestbookrepo.PageResult, resolver storage.ObjectURLResolver) *dto.GuestbookPageResp {
	items := make([]dto.GuestbookItemResp, 0, len(result.Messages))
	for _, message := range result.Messages {
		items = append(items, *guestbookItemToDTO(message, resolver))
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

func guestbookItemToDTO(aggregate guestbookrepo.GuestbookAggregate, resolver storage.ObjectURLResolver) *dto.GuestbookItemResp {
	message := aggregate.Message
	return &dto.GuestbookItemResp{
		ID:          message.ID,
		OwnerUserID: message.OwnerUserID,
		FromUserID:  message.FromUserID,
		Content:     message.Content,
		User:        guestbookUserToDTO(aggregate.User, resolver),
		ReplyCount:  aggregate.ReplyCount,
		LikeCount:   aggregate.LikeCount,
		IsLiked:     aggregate.IsLiked,
		CreatedAt:   message.CreatedAt,
		UpdatedAt:   message.UpdatedAt,
	}
}

func guestbookUserToDTO(user *model.User, resolver storage.ObjectURLResolver) *dto.GuestbookUserResp {
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
	}
}
