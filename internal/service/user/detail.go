package user

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	userrepo "github.com/vpt/blog-backend/internal/repository/user"
	"github.com/vpt/blog-backend/pkg/storage"
)

// ErrUserNotFound 表示当前 token 对应的用户已不存在。
var ErrUserNotFound = errors.New("用户不存在")

// assembleUserDetail 将 DB 聚合模型转换为对外响应 DTO，供 UserCacheService 调用。
func assembleUserDetail(resolver storage.ObjectURLResolver, aggregate *userrepo.UserDetailAggregate) *dto.UserDetailResp {
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
	resp := make([]dto.UserSocialLinkResp, 0, len(links))
	for _, link := range links {
		resp = append(resp, dto.UserSocialLinkResp{
			Platform: link.Platform,
			URL:      link.URL,
		})
	}
	return resp
}

// buildPublicProfile 将 DB 聚合模型转换为公开详情 DTO，隐藏私密字段。
func buildPublicProfile(resolver storage.ObjectURLResolver, agg *userrepo.UserDetailAggregate) *dto.UserPublicProfileResp {
	user := agg.User
	nickname := user.Username
	if user.Nickname != nil && *user.Nickname != "" {
		nickname = *user.Nickname
	}

	resp := &dto.UserPublicProfileResp{
		ID:          user.ID,
		Nickname:    nickname,
		AvatarUrl:   resolveUserAvatarURL(resolver, user.AvatarUrl),
		Mark:        user.Mark,
		Site:        user.Site,
		LastLoginAt: user.LastLoginAt,
		RegisterAt:  user.CreatedAt,
		Roles:       append([]string(nil), agg.Roles...),
		SocialLinks: userSocialLinksToDTO(agg.SocialLinks),
	}

	if agg.Meta != nil {
		resp.Description = agg.Meta.Description
		if agg.Meta.Gender != nil {
			g := fmt.Sprintf("%d", *agg.Meta.Gender)
			resp.Gender = &g
		}
		if agg.Meta.Birthday != nil {
			b := agg.Meta.Birthday.Format("2006-01-02")
			resp.Birthday = &b
		}
	}

	// 根据邮箱展示设置决定对外显示哪个邮箱
	if agg.Setting != nil {
		switch agg.Setting.MailShow {
		case 1:
			resp.DisplayEmail = user.Email
		// case 0: sub email（暂无字段，留空）
		// case 2: none（不展示）
		}
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
