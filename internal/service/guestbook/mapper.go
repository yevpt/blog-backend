package guestbook

import (
	"math"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	guestbookrepo "github.com/vpt/blog-backend/internal/repository/guestbook"
)

func guestbookPageToDTO(result *guestbookrepo.PageResult) *dto.GuestbookPageResp {
	items := make([]dto.GuestbookItemResp, 0, len(result.Messages))
	for _, message := range result.Messages {
		items = append(items, *guestbookItemToDTO(message))
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

func guestbookItemToDTO(aggregate guestbookrepo.GuestbookAggregate) *dto.GuestbookItemResp {
	message := aggregate.Message
	return &dto.GuestbookItemResp{
		ID:          message.ID,
		OwnerUserID: message.OwnerUserID,
		FromUserID:  message.FromUserID,
		Content:     message.Content,
		User:        guestbookUserToDTO(aggregate.User),
		ReplyCount:  aggregate.ReplyCount,
		LikeCount:   aggregate.LikeCount,
		IsLiked:     aggregate.IsLiked,
		CreatedAt:   message.CreatedAt,
		UpdatedAt:   message.UpdatedAt,
	}
}

func guestbookUserToDTO(user *model.User) *dto.GuestbookUserResp {
	if user == nil {
		return nil
	}
	return &dto.GuestbookUserResp{
		ID:        user.ID,
		Username:  user.Username,
		Nickname:  user.Nickname,
		AvatarUrl: user.AvatarUrl,
		Site:      user.Site,
		Mark:      user.Mark,
	}
}
