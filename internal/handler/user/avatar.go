package user

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/imageupload"
	"github.com/vpt/blog-backend/internal/handler/multipartlimit"
	"github.com/vpt/blog-backend/internal/middleware"
	avatarservice "github.com/vpt/blog-backend/internal/service/avatar"
	"github.com/vpt/blog-backend/pkg/response"
)

// UploadAvatar 上传并更换当前用户头像。
// @Summary 更换当前用户头像
// @Description 登录用户上传头像图片，服务端校验后入库：最长边 ≤240px、体积 ≤20KB；已合规的 JPG/PNG/WebP 原样保留，超出时压缩为 WebP 并更新 avatar_url；不支持 GIF，原始文件最大 256KB。
// @Tags 用户
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "头像图片（JPG、PNG、WebP）"
// @Success 200 {object} response.Response{data=dto.UserDetailResp} "统一响应；code=0 表示更换成功，code=400 表示参数错误或业务错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 429 {object} response.Response "请求过于频繁"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /users/me/avatar [post]
func (h *UserHandler) UploadAvatar(c *gin.Context) {
	detail := middleware.GetUserDetail(c)
	if detail == nil {
		response.Unauthorized(c)
		return
	}

	name, data, err := imageupload.ReadSingleImageFile(c, "file", avatarservice.MaxRawAvatarBytes, true)
	if errors.Is(err, multipartlimit.ErrBodyTooLarge) || errors.Is(err, multipartlimit.ErrTooManyFiles) {
		return
	}
	if err != nil {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}
	if len(data) == 0 {
		response.Fail(c, response.CodeBadRequest, "缺少上传文件")
		return
	}

	resp, err := h.svc.ChangeAvatar(detail.ID, &dto.UploadedImageFile{
		Name: name,
		Data: data,
	})
	if err != nil {
		writeAvatarError(c, err)
		return
	}
	response.Success(c, resp)
}

func writeAvatarError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, avatarservice.ErrAvatarTooLarge),
		errors.Is(err, avatarservice.ErrAvatarInvalid),
		errors.Is(err, avatarservice.ErrAvatarGIFNotAllowed),
		errors.Is(err, avatarservice.ErrAvatarCompressedTooLarge):
		response.Fail(c, response.CodeBadRequest, err.Error())
	default:
		response.ServerError(c)
	}
}
