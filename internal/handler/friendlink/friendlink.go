package friendlink

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
	friendlinkservice "github.com/vpt/blog-backend/internal/service/friendlink"
	"github.com/vpt/blog-backend/pkg/response"
)

// FriendLinkHandler 友情链接模块 HTTP 入口。
type FriendLinkHandler struct {
	svc friendlinkservice.FriendLinkService
}

// NewFriendLinkHandler 创建友情链接 HTTP handler。
func NewFriendLinkHandler(svc friendlinkservice.FriendLinkService) *FriendLinkHandler {
	return &FriendLinkHandler{svc: svc}
}

// ListPublic 查询公开友情链接列表。
// @Summary 查询公开友情链接列表
// @Description 分页返回显示中的友情链接，按 seq ASC、id DESC 排序；avatar_url 返回前会解析为可访问 URL。
// @Tags 友情链接
// @Produce json
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，最大 50"
// @Success 200 {object} response.Response{data=dto.FriendLinkPageResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /friend-links [get]
func (h *FriendLinkHandler) ListPublic(c *gin.Context) {
	var req dto.FriendLinkListReq
	if !reqbind.Query(c, &req) {
		return
	}
	resp, err := h.svc.ListPublic(req)
	writeFriendLinkResponse(c, resp, err)
}

// GetPublic 查询公开友情链接详情。
// @Summary 查询公开友情链接详情
// @Description 查询显示中的友情链接详情；隐藏或不存在的数据都会返回 404。
// @Tags 友情链接
// @Produce json
// @Param id path int true "友情链接 ID"
// @Success 200 {object} response.Response{data=dto.FriendLinkItemResp} "统一响应；code=0 表示查询成功"
// @Failure 404 {object} response.Response "友情链接不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /friend-links/{id} [get]
func (h *FriendLinkHandler) GetPublic(c *gin.Context) {
	id, ok := bindFriendLinkID(c)
	if !ok {
		return
	}
	resp, err := h.svc.GetPublic(id)
	writeFriendLinkResponse(c, resp, err)
}

// ListAdmin 查询管理端友情链接列表。
// @Summary 查询管理端友情链接列表
// @Description 管理员分页查询未删除的友情链接，可按 status 过滤。
// @Tags 友情链接
// @Produce json
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，最大 50"
// @Param status query int false "状态：0 隐藏，1 显示"
// @Success 200 {object} response.Response{data=dto.FriendLinkPageResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/friend-links [get]
func (h *FriendLinkHandler) ListAdmin(c *gin.Context) {
	var req dto.FriendLinkListReq
	if !reqbind.Query(c, &req) {
		return
	}
	resp, err := h.svc.ListAdmin(req)
	writeFriendLinkResponse(c, resp, err)
}

// Create 新增友情链接。
// @Summary 新增友情链接
// @Description 管理员新增友情链接；必须随表单上传 logo 文件，服务端保存为 avatar/link/{md5}.jpg 并写入 avatar_url。
// @Tags 友情链接
// @Accept multipart/form-data
// @Produce json
// @Param name formData string true "网站名称"
// @Param description formData string false "网站描述"
// @Param email formData string false "站长邮箱"
// @Param phone formData string false "联系电话"
// @Param site formData string true "网站 URL"
// @Param seq formData int true "排序值，越小越靠前"
// @Param status formData int false "状态：0 隐藏，1 显示，2 失联；未传默认显示"
// @Param logo formData file true "友链 Logo 图片（JPG、PNG、WebP），最大 2MB"
// @Success 200 {object} response.Response{data=dto.FriendLinkItemResp} "统一响应；code=0 表示新增成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 429 {object} response.Response "请求过于频繁"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/friend-links [post]
func (h *FriendLinkHandler) Create(c *gin.Context) {
	req, ok := bindFriendLinkCreateReq(c)
	if !ok {
		return
	}
	resp, err := h.svc.Create(*req)
	writeFriendLinkResponse(c, resp, err)
}

// Update 修改友情链接。
// @Summary 修改友情链接
// @Description 管理员修改友情链接；必须随表单上传 logo 文件，服务端保存为 avatar/link/{md5}.jpg 并替换 avatar_url；未传文本字段保持原值，可选字符串传空字符串表示清空。
// @Tags 友情链接
// @Accept multipart/form-data
// @Produce json
// @Param id path int true "友情链接 ID"
// @Param name formData string false "网站名称"
// @Param description formData string false "网站描述；传空字符串表示清空"
// @Param email formData string false "站长邮箱；传空字符串表示清空"
// @Param phone formData string false "联系电话；传空字符串表示清空"
// @Param site formData string false "网站 URL"
// @Param seq formData int false "排序值，越小越靠前"
// @Param status formData int false "状态：0 隐藏，1 显示，2 失联"
// @Param logo formData file true "友链 Logo 图片（JPG、PNG、WebP），最大 2MB"
// @Success 200 {object} response.Response{data=dto.FriendLinkItemResp} "统一响应；code=0 表示修改成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 404 {object} response.Response "友情链接不存在"
// @Failure 429 {object} response.Response "请求过于频繁"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/friend-links/{id} [put]
func (h *FriendLinkHandler) Update(c *gin.Context) {
	id, ok := bindFriendLinkID(c)
	if !ok {
		return
	}
	req, ok := bindFriendLinkUpdateReq(c)
	if !ok {
		return
	}
	resp, err := h.svc.Update(id, *req)
	writeFriendLinkResponse(c, resp, err)
}

// Delete 删除友情链接。
// @Summary 删除友情链接
// @Description 管理员软删除友情链接。
// @Tags 友情链接
// @Produce json
// @Param id path int true "友情链接 ID"
// @Success 200 {object} response.Response{data=dto.FriendLinkItemResp} "统一响应；code=0 表示删除成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 404 {object} response.Response "友情链接不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/friend-links/{id} [delete]
func (h *FriendLinkHandler) Delete(c *gin.Context) {
	id, ok := bindFriendLinkID(c)
	if !ok {
		return
	}
	resp, err := h.svc.Delete(id)
	writeFriendLinkResponse(c, resp, err)
}

func bindFriendLinkID(c *gin.Context) (uint, bool) {
	return reqbind.PathUint(c, "id", "友情链接 ID")
}

func bindFriendLinkCreateReq(c *gin.Context) (*dto.FriendLinkCreateReq, bool) {
	var req dto.FriendLinkCreateReq
	if !reqbind.Form(c, &req) {
		return nil, false
	}
	logo, ok := readFriendLinkLogo(c)
	if !ok {
		return nil, false
	}
	req.Logo = logo
	return &req, true
}

func bindFriendLinkUpdateReq(c *gin.Context) (*dto.FriendLinkUpdateReq, bool) {
	var req dto.FriendLinkUpdateReq
	if !reqbind.Form(c, &req) {
		return nil, false
	}
	logo, ok := readFriendLinkLogo(c)
	if !ok {
		return nil, false
	}
	req.Logo = logo
	return &req, true
}

func readFriendLinkLogo(c *gin.Context) (*dto.UploadedImageFile, bool) {
	header, err := c.FormFile("logo")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			response.Fail(c, response.CodeBadRequest, friendlinkservice.ErrFriendLinkLogoRequired.Error())
			return nil, false
		}
		response.Fail(c, response.CodeBadRequest, "读取友链 Logo 失败")
		return nil, false
	}

	file, err := header.Open()
	if err != nil {
		response.Fail(c, response.CodeBadRequest, "读取友链 Logo 失败")
		return nil, false
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, int64(friendlinkservice.MaxFriendLinkLogoBytes)+1))
	if err != nil {
		response.Fail(c, response.CodeBadRequest, "读取友链 Logo 失败")
		return nil, false
	}
	if len(data) > friendlinkservice.MaxFriendLinkLogoBytes {
		response.Fail(c, response.CodeBadRequest, friendlinkservice.ErrFriendLinkLogoTooLarge.Error())
		return nil, false
	}
	return &dto.UploadedImageFile{Name: header.Filename, Data: data}, true
}

func writeFriendLinkResponse(c *gin.Context, data any, err error) {
	if err == nil {
		response.Success(c, data)
		return
	}
	if errors.Is(err, friendlinkservice.ErrFriendLinkNotFound) {
		response.NotFound(c)
		return
	}
	if isFriendLinkBadRequest(err) {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}
	response.ServerError(c)
}

func isFriendLinkBadRequest(err error) bool {
	return errors.Is(err, friendlinkservice.ErrFriendLinkNameRequired) ||
		errors.Is(err, friendlinkservice.ErrFriendLinkSiteRequired) ||
		errors.Is(err, friendlinkservice.ErrFriendLinkSeqRequired) ||
		errors.Is(err, friendlinkservice.ErrFriendLinkStatusInvalid) ||
		errors.Is(err, friendlinkservice.ErrFriendLinkLogoRequired) ||
		errors.Is(err, friendlinkservice.ErrFriendLinkLogoInvalid) ||
		errors.Is(err, friendlinkservice.ErrFriendLinkLogoTooLarge) ||
		errors.Is(err, friendlinkservice.ErrFriendLinkLogoGIF) ||
		errors.Is(err, friendlinkservice.ErrFriendLinkLogoStoredBig)
}
