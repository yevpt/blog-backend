package upload

import (
	"errors"
	"io"

	"github.com/gin-gonic/gin"
	uploadservice "github.com/vpt/blog-backend/internal/service/upload"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

type Handler struct {
	svc uploadservice.Service
}

func NewHandler(svc uploadservice.Service) *Handler {
	return &Handler{svc: svc}
}

// TempImage 上传临时图片。
// @Summary 上传临时图片
// @Description 登录用户上传临时图片。默认 scene=article，仅支持 images/covers/mobile-covers；scene=comment 用于留言、评论、回复图片，仅支持 images，普通图片最大 1MB 并压缩到 500KB 内，GIF 最大 300KB。
// @Tags 上传
// @Accept multipart/form-data
// @Produce json
// @Param scene formData string false "上传场景：article 或 comment；默认 article"
// @Param dir formData string true "临时目录：images、covers 或 mobile-covers"
// @Param file formData file true "图片文件；article 最大 10MB，comment 普通图片最大 1MB、GIF 最大 300KB"
// @Success 200 {object} response.Response{data=dto.TempUploadResp} "统一响应；code=0 表示上传成功，code=400 表示参数错误或业务错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 429 {object} response.Response "请求过于频繁"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /uploads/temp [post]
func (h *Handler) TempImage(c *gin.Context) {
	claims := jwtpkg.GetClaims(c)
	if claims == nil || claims.UserId <= 0 {
		response.Unauthorized(c)
		return
	}

	scene := c.PostForm("scene")
	dir := c.PostForm("dir")
	header, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, response.CodeBadRequest, "缺少上传文件")
		return
	}
	file, err := header.Open()
	if err != nil {
		response.Fail(c, response.CodeBadRequest, "读取上传文件失败")
		return
	}
	defer file.Close()

	readLimit := uploadservice.MaxTempUploadReadBytes(scene)
	data, err := io.ReadAll(io.LimitReader(file, int64(readLimit)+1))
	if err != nil {
		response.Fail(c, response.CodeBadRequest, "读取上传文件失败")
		return
	}
	if len(data) > readLimit {
		response.Fail(c, response.CodeBadRequest, tempUploadTooLargeMessage(scene))
		return
	}

	resp, err := h.svc.UploadTempImage(c.Request.Context(), uploadservice.TempImageInput{
		UserID: uint(claims.UserId),
		Scene:  scene,
		Dir:    dir,
		Name:   header.Filename,
		Data:   data,
	})
	if err != nil {
		switch {
		case errors.Is(err, uploadservice.ErrUploadTooLarge),
			errors.Is(err, uploadservice.ErrUploadCommentTooLarge),
			errors.Is(err, uploadservice.ErrUploadCommentGIFLarge),
			errors.Is(err, uploadservice.ErrUploadCompressedLarge):
			response.Fail(c, response.CodeBadRequest, err.Error())
		case errors.Is(err, uploadservice.ErrUploadDirInvalid),
			errors.Is(err, uploadservice.ErrUploadSceneInvalid),
			errors.Is(err, uploadservice.ErrUploadInvalid):
			response.Fail(c, response.CodeBadRequest, err.Error())
		case errors.Is(err, uploadservice.ErrUploadForbidden):
			response.ForbiddenWithMessage(c, err.Error())
		default:
			response.ServerError(c)
		}
		return
	}
	response.Success(c, resp)
}

func tempUploadTooLargeMessage(scene string) string {
	if uploadservice.MaxTempUploadReadBytes(scene) == uploadservice.MaxCommentTempImageBytes {
		return uploadservice.ErrUploadCommentTooLarge.Error()
	}
	return uploadservice.ErrUploadTooLarge.Error()
}
