package moment

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
	"github.com/vpt/blog-backend/pkg/response"
)

const (
	maxMomentImageUploadBytes = 3 * 1024 * 1024
	maxMomentGifUploadBytes   = 300 * 1024
)

// Save 新增或更新碎语。
// @Summary 新增或更新碎语
// @Description 登录用户新增或更新自己的碎语；管理员可通过 user_id 指定作者。image_urls 传已上传图片 URL/key，images 传新图片文件，图片会整体替换。
// @Tags 碎语
// @Accept multipart/form-data
// @Produce json
// @Param id formData int false "碎语 ID；为空表示新增"
// @Param user_id formData int false "作者用户 ID；仅管理员可代管"
// @Param content formData string true "碎语正文"
// @Param status formData int true "状态：0 隐藏，1 公开"
// @Param comment_status formData int true "评论状态：0 关闭，1 开启"
// @Param image_urls formData []string false "已上传图片对象 key 或 URL，可重复传入"
// @Param image_order formData []string false "图片顺序引用，可重复传入；url:N 指向第 N 个 image_urls，file:N 指向第 N 个 images"
// @Param images formData file false "新图片文件，可重复传入"
// @Param Idempotency-Key header string true "幂等键，重试时保持不变"
// @Success 200 {object} response.Response{data=dto.MomentItemResp} "统一响应；code=0 表示保存成功，code=400 表示参数错误或业务错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "无权操作碎语"
// @Failure 404 {object} response.Response "碎语或作者不存在"
// @Failure 409 {object} response.Response "图片审核未启用或内容状态冲突"
// @Failure 422 {object} response.Response "内容存在风险，拒绝发布"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments [post]
func (h *MomentHandler) Save(c *gin.Context) {
	userID, roleNames, ok := requiredUser(c)
	if !ok {
		return
	}
	key, ok := reqbind.IdempotencyKeyIf(c, h.requireModerationWrite)
	if !ok {
		return
	}

	req, ok := bindMomentSaveReq(c)
	if !ok {
		return
	}
	req.IdempotencyKey = key

	resp, err := h.svc.Save(*req, userID, roleNames)
	writeMomentResponse(c, resp, err)
}

func bindMomentSaveReq(c *gin.Context) (*dto.MomentSaveReq, bool) {
	var req dto.MomentSaveReq
	if !reqbind.Form(c, &req) {
		return nil, false
	}

	files, err := readMomentImageFiles(c)
	if err != nil {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return nil, false
	}
	req.ImageFiles = files
	return &req, true
}

func readMomentImageFiles(c *gin.Context) ([]dto.MomentImageFileReq, error) {
	form, err := c.MultipartForm()
	if err != nil {
		return nil, err
	}
	headers := form.File["images"]
	files := make([]dto.MomentImageFileReq, 0, len(headers))
	for _, header := range headers {
		file, err := header.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxMomentImageUploadBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		contentType := header.Header.Get("Content-Type")
		if contentType == "" || contentType == "application/octet-stream" {
			contentType = http.DetectContentType(data)
		}
		if isMomentGifUpload(contentType, header.Filename) && len(data) > maxMomentGifUploadBytes {
			return nil, errors.New("GIF 图片过大，暂不支持压缩该格式，请上传 300KB 以内的 GIF。")
		}
		if len(data) > maxMomentImageUploadBytes {
			return nil, errors.New("图片不能超过 3MB")
		}
		files = append(files, dto.MomentImageFileReq{
			Name:        header.Filename,
			ContentType: contentType,
			Data:        data,
		})
	}
	return files, nil
}

func isMomentGifUpload(contentType string, fileName string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return mediaType == "image/gif" || strings.EqualFold(filepath.Ext(fileName), ".gif")
}

// Delete 删除碎语。
// @Summary 删除碎语
// @Description 碎语作者或管理员可硬删除碎语；同时级联硬删除关联的媒体、评论、回复、点赞和通知消息，并清理 Garage 对象。
// @Tags 碎语
// @Accept json
// @Produce json
// @Param id path int true "碎语 ID"
// @Success 200 {object} response.Response{data=dto.MomentDeleteResp} "统一响应；code=0 表示删除成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "无权操作碎语"
// @Failure 404 {object} response.Response "碎语不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments/{id} [delete]
func (h *MomentHandler) Delete(c *gin.Context) {
	userID, roleNames, ok := requiredUser(c)
	if !ok {
		return
	}
	id, ok := bindMomentID(c, "id")
	if !ok {
		return
	}

	resp, err := h.svc.Delete(id, userID, roleNames)
	writeMomentResponse(c, resp, err)
}

// SetTop 置顶碎语。
// @Summary 置顶碎语
// @Description 碎语作者或管理员可置顶碎语；每个作者最多置顶三条。
// @Tags 碎语
// @Accept json
// @Produce json
// @Param id path int true "碎语 ID"
// @Success 200 {object} response.Response{data=dto.MomentTopResp} "统一响应；code=0 表示置顶成功，code=400 表示参数错误或达到上限"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "无权操作碎语"
// @Failure 404 {object} response.Response "碎语不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments/{id}/top [post]
func (h *MomentHandler) SetTop(c *gin.Context) {
	userID, roleNames, ok := requiredUser(c)
	if !ok {
		return
	}
	id, ok := bindMomentID(c, "id")
	if !ok {
		return
	}

	resp, err := h.svc.SetTop(id, userID, roleNames)
	writeMomentResponse(c, resp, err)
}

// RemoveTop 取消置顶碎语。
// @Summary 取消置顶碎语
// @Description 碎语作者或管理员可取消置顶碎语。
// @Tags 碎语
// @Accept json
// @Produce json
// @Param id path int true "碎语 ID"
// @Success 200 {object} response.Response{data=dto.MomentTopResp} "统一响应；code=0 表示取消成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "无权操作碎语"
// @Failure 404 {object} response.Response "碎语不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments/{id}/top [delete]
func (h *MomentHandler) RemoveTop(c *gin.Context) {
	userID, roleNames, ok := requiredUser(c)
	if !ok {
		return
	}
	id, ok := bindMomentID(c, "id")
	if !ok {
		return
	}

	resp, err := h.svc.RemoveTop(id, userID, roleNames)
	writeMomentResponse(c, resp, err)
}

// ToggleLike 切换碎语点赞状态。
// @Summary 切换碎语点赞
// @Description 当前用户未点赞时点赞，已点赞时取消点赞。
// @Tags 碎语
// @Accept json
// @Produce json
// @Param id path int true "碎语 ID"
// @Success 200 {object} response.Response{data=dto.MomentItemResp} "统一响应；code=0 表示切换成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "碎语不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments/{id}/like [post]
func (h *MomentHandler) ToggleLike(c *gin.Context) {
	userID, _, ok := requiredUser(c)
	if !ok {
		return
	}
	id, ok := bindMomentID(c, "id")
	if !ok {
		return
	}

	resp, err := h.svc.ToggleLike(id, userID)
	writeMomentResponse(c, resp, err)
}
