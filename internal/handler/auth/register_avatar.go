package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/imageupload"
	avatarservice "github.com/vpt/blog-backend/internal/service/avatar"
)

func readRegisterAvatar(c *gin.Context) (*dto.UploadedImageFile, error) {
	name, data, err := imageupload.ReadSingleImageFile(c, "avatar", avatarservice.MaxRawAvatarBytes, true)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	return &dto.UploadedImageFile{
		Name: name,
		Data: data,
	}, nil
}
