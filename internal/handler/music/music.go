package music

import (
	"github.com/gin-gonic/gin"
	musicservice "github.com/vpt/blog-backend/internal/service/music"
	"github.com/vpt/blog-backend/pkg/response"
)

// MusicHandler 音乐模块 HTTP 入口。
type MusicHandler struct {
	svc musicservice.MusicService
}

// NewMusicHandler 创建音乐 HTTP handler。
func NewMusicHandler(svc musicservice.MusicService) *MusicHandler {
	return &MusicHandler{svc: svc}
}

// List 查询音乐列表。
// @Summary 查询音乐列表
// @Description 返回所有未删除音乐，按 seq ASC、id ASC 排序；url 和 cover_img_url 返回前会解析为可访问 URL。
// @Tags 音乐
// @Produce json
// @Success 200 {object} response.Response{data=dto.MusicListResp} "统一响应；code=0 表示查询成功"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /music [get]
func (h *MusicHandler) List(c *gin.Context) {
	resp, err := h.svc.List()
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
}
