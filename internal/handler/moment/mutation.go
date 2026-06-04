package moment

import (
	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
)

// Save 新增或更新碎语。
// @Summary 新增或更新碎语
// @Description 登录用户新增或更新自己的碎语；管理员可通过 user_id 指定作者，并同步图片列表。
// @Tags 碎语
// @Accept json
// @Produce json
// @Param request body dto.MomentSaveReq true "碎语保存请求"
// @Success 200 {object} response.Response{data=dto.MomentItemResp} "统一响应；code=0 表示保存成功，code=400 表示参数错误或业务错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "无权操作碎语"
// @Failure 404 {object} response.Response "碎语或作者不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments [post]
func (h *MomentHandler) Save(c *gin.Context) {
	userID, roleNames, ok := requiredUser(c)
	if !ok {
		return
	}

	var req dto.MomentSaveReq
	if !reqbind.JSON(c, &req) {
		return
	}

	resp, err := h.svc.Save(req, userID, roleNames)
	writeMomentResponse(c, resp, err)
}

// Delete 删除碎语。
// @Summary 删除碎语
// @Description 碎语作者或管理员可软删除碎语。
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
