package service

import (
	"context"
	"errors"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/repository"
	"github.com/vpt/blog-backend/pkg/storage"
)

// ErrUserNotFound 表示当前 token 对应的用户已不存在。
var ErrUserNotFound = errors.New("用户不存在")

// assembleUserDetail 将 DB 聚合模型转换为对外响应 DTO，供 UserCacheService 调用。
func assembleUserDetail(resolver storage.ObjectURLResolver, aggregate *repository.UserDetailAggregate) *dto.UserDetailResp {
	user := aggregate.User
	resp := &dto.UserDetailResp{
		ID:          user.ID,
		Username:    user.Username,
		Nickname:    user.Nickname,
		Email:       user.Email,
		Phone:       user.Phone,
		Site:        user.Site,
		AvatarUrl:   resolveUserAvatarURL(resolver, user.AvatarUrl),
		Mark:        user.Mark,
		Status:      user.Status,
		LastLoginAt: user.LastLoginAt,
		Roles:       append([]string(nil), aggregate.Roles...),
		Meta:        userMetaToDTO(aggregate.Meta),
		Setting:     userSettingToDTO(aggregate.Setting),
		SocialLinks: userSocialLinksToDTO(aggregate.SocialLinks),
	}
	return resp
}

func userMetaToDTO(meta *model.UserMeta) *dto.UserMetaResp {
	if meta == nil {
		return nil
	}
	return &dto.UserMetaResp{
		Name:        meta.Name,
		Description: meta.Description,
		Gender:      meta.Gender,
		Birthday:    meta.Birthday,
		Country:     meta.Country,
		Province:    meta.Province,
		City:        meta.City,
		Address:     meta.Address,
	}
}

func userSettingToDTO(setting *model.UserSetting) *dto.UserSettingResp {
	if setting == nil {
		return nil
	}
	return &dto.UserSettingResp{
		MailShow:     setting.MailShow,
		MailReceive:  setting.MailReceive,
		DarkMode:     setting.DarkMode,
		ReceiveMail:  setting.ReceiveMail,
		ShowName:     setting.ShowName,
		ShowAge:      setting.ShowAge,
		ShowPhone:    setting.ShowPhone,
		ShowQq:       setting.ShowQq,
		ShowWechat:   setting.ShowWechat,
		ShowZhihu:    setting.ShowZhihu,
		ShowSina:     setting.ShowSina,
		ShowBili:     setting.ShowBili,
		ShowPosition: setting.ShowPosition,
	}
}

func userSocialLinksToDTO(links []model.UserSocialLink) []dto.UserSocialLinkResp {
	if len(links) == 0 {
		return nil
	}
	resp := make([]dto.UserSocialLinkResp, 0, len(links))
	for _, link := range links {
		resp = append(resp, dto.UserSocialLinkResp{
			Platform: link.Platform,
			URL:      link.URL,
		})
	}
	return resp
}

func resolveUserAvatarURL(resolver storage.ObjectURLResolver, url *string) *string {
	if url == nil || resolver == nil {
		return url
	}
	trimmed := strings.TrimSpace(*url)
	if trimmed == "" || strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return url
	}
	if resolved, err := resolver.ObjectURL(context.Background(), trimmed); err == nil {
		return &resolved
	}
	return url
}
