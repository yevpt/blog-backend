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

// TempImage 上传文章临时图片。
// @Summary 上传文章临时图片
// @Description 登录用户上传文章编辑阶段的临时图片，仅支持 images/covers 目录，服务端按内容哈希去重并返回对象 key 与访问 URL。
// @Tags 上传
// @Accept multipart/form-data
// @Produce json
// @Param dir formData string true "临时目录：images 或 covers"
// @Param file formData file true "图片文件，最大 10MB"
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

	data, err := io.ReadAll(io.LimitReader(file, uploadservice.MaxTempImageBytes+1))
	if err != nil {
		response.Fail(c, response.CodeBadRequest, "读取上传文件失败")
		return
	}
	if len(data) > uploadservice.MaxTempImageBytes {
		response.Fail(c, response.CodeBadRequest, uploadservice.ErrUploadTooLarge.Error())
		return
	}

	resp, err := h.svc.UploadTempImage(c.Request.Context(), uploadservice.TempImageInput{
		UserID: uint(claims.UserId),
		Dir:    dir,
		Name:   header.Filename,
		Data:   data,
	})
	if err != nil {
		switch {
		case errors.Is(err, uploadservice.ErrUploadTooLarge):
			response.Fail(c, response.CodeBadRequest, err.Error())
		case errors.Is(err, uploadservice.ErrUploadDirInvalid), errors.Is(err, uploadservice.ErrUploadInvalid):
			response.Fail(c, response.CodeBadRequest, err.Error())
		default:
			response.ServerError(c)
		}
		return
	}
	response.Success(c, resp)
}
