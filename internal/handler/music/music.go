package music

import (
	"errors"
	"io"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
	musicservice "github.com/vpt/blog-backend/internal/service/music"
	uploadservice "github.com/vpt/blog-backend/internal/service/upload"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
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

// List 查询公开音乐列表。
// @Summary 查询公开音乐列表
// @Description 返回 is_public=true 的未删除音乐，按 seq ASC、id ASC 排序；audio_url 和 cover_url 返回前会解析为可访问 URL。
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

// GetPublicDetail 查询公开音乐详情。
// @Summary 查询公开音乐详情
// @Description 返回单首公开音乐的详情；非公开或不存在的数据返回 404。
// @Tags 音乐
// @Produce json
// @Param id path int true "音乐 ID"
// @Success 200 {object} response.Response{data=dto.MusicDetailResp} "统一响应；code=0 表示查询成功"
// @Failure 400 {object} response.Response "参数错误"
// @Failure 404 {object} response.Response "音乐不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /music/{id} [get]
func (h *MusicHandler) GetPublicDetail(c *gin.Context) {
	id, ok := parseMusicID(c)
	if !ok {
		return
	}
	resp, err := h.svc.GetPublicDetail(id)
	if err != nil {
		if errors.Is(err, musicservice.ErrMusicNotFound) {
			response.NotFound(c)
			return
		}
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
}

// ListArtists 查询公开歌手列表。
// @Summary 查询公开歌手列表
// @Description 返回歌手列表，可按 name 或 name_zh 关键字过滤；avatar_url 返回前会解析为可访问 URL。
// @Tags 音乐
// @Produce json
// @Param keyword query string false "搜索关键字"
// @Success 200 {object} response.Response{data=dto.MusicArtistListResp} "统一响应；code=0 表示查询成功"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /music/artists [get]
func (h *MusicHandler) ListArtists(c *gin.Context) {
	resp, err := h.svc.ListArtists(c.Query("keyword"))
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
}

// GetPublicArtist 查询公开歌手详情。
// @Summary 查询公开歌手详情
// @Description 返回单个歌手详情；不存在时返回 404。
// @Tags 音乐
// @Produce json
// @Param id path int true "歌手 ID"
// @Success 200 {object} response.Response{data=dto.MusicArtistResp} "统一响应；code=0 表示查询成功"
// @Failure 400 {object} response.Response "参数错误"
// @Failure 404 {object} response.Response "歌手不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /music/artists/{id} [get]
func (h *MusicHandler) GetPublicArtist(c *gin.Context) {
	id, ok := parseMusicID(c)
	if !ok {
		return
	}
	resp, err := h.svc.GetPublicArtist(id)
	if err != nil {
		if errors.Is(err, musicservice.ErrMusicArtistNotFound) {
			response.NotFound(c)
			return
		}
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
}

// ListAlbums 查询公开专辑列表。
// @Summary 查询公开专辑列表
// @Description 返回专辑列表，可按专辑名关键字过滤；cover_url 返回前会解析为可访问 URL。
// @Tags 音乐
// @Produce json
// @Param keyword query string false "搜索关键字"
// @Success 200 {object} response.Response{data=dto.MusicAlbumListResp} "统一响应；code=0 表示查询成功"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /music/albums [get]
func (h *MusicHandler) ListAlbums(c *gin.Context) {
	resp, err := h.svc.ListAlbums(c.Query("keyword"))
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
}

// GetPublicAlbum 查询公开专辑详情。
// @Summary 查询公开专辑详情
// @Description 返回单个专辑详情；不存在时返回 404。
// @Tags 音乐
// @Produce json
// @Param id path int true "专辑 ID"
// @Success 200 {object} response.Response{data=dto.MusicAlbumResp} "统一响应；code=0 表示查询成功"
// @Failure 400 {object} response.Response "参数错误"
// @Failure 404 {object} response.Response "专辑不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /music/albums/{id} [get]
func (h *MusicHandler) GetPublicAlbum(c *gin.Context) {
	id, ok := parseMusicID(c)
	if !ok {
		return
	}
	resp, err := h.svc.GetPublicAlbum(id)
	if err != nil {
		if errors.Is(err, musicservice.ErrMusicAlbumNotFound) {
			response.NotFound(c)
			return
		}
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
}

// ListAdmin 分页查询管理端音乐列表。
// @Summary 分页查询管理端音乐列表
// @Description 管理员分页查询未删除音乐，可按曲名或歌手展示名关键字过滤。
// @Tags 音乐
// @Produce json
// @Param keyword query string false "搜索关键字"
// @Param page query int false "页码，从 1 开始，默认 1"
// @Param page_size query int false "每页数量，默认 20，最大 100"
// @Success 200 {object} response.Response{data=dto.MusicAdminListResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/music [get]
func (h *MusicHandler) ListAdmin(c *gin.Context) {
	var req dto.MusicAdminListReq
	if !reqbind.Query(c, &req) {
		return
	}
	normalizeMusicAdminListReq(&req)

	resp, err := h.svc.ListAdmin(req)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
}

// SaveMusic 新增或修改音乐。
// @Summary 新增或修改音乐
// @Description 管理员新增或修改音乐；PUT 时路径 ID 优先于请求体中的 id。
// @Tags 音乐
// @Accept json
// @Produce json
// @Param id path int false "音乐 ID（仅 PUT）"
// @Param request body dto.MusicSaveReq true "音乐保存请求"
// @Success 200 {object} response.Response "统一响应；code=0 表示保存成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/music [post]
// @Router /admin/music/{id} [put]
func (h *MusicHandler) SaveMusic(c *gin.Context) {
	var req dto.MusicSaveReq
	if !reqbind.JSON(c, &req) {
		return
	}
	if idStr := c.Param("id"); idStr != "" {
		id, ok := parseMusicID(c)
		if !ok {
			return
		}
		req.ID = id
	}
	claims := jwtpkg.GetClaims(c)
	if claims == nil || claims.UserId <= 0 {
		response.Unauthorized(c)
		return
	}
	if err := h.svc.SaveMusic(c.Request.Context(), uint(claims.UserId), req); err != nil {
		if errors.Is(err, musicservice.ErrMusicArtistNotFound) ||
			errors.Is(err, musicservice.ErrMusicAlbumNotFound) ||
			errors.Is(err, musicservice.ErrMusicAssetInvalid) ||
			errors.Is(err, musicservice.ErrMusicAssetNotFound) {
			response.Fail(c, response.CodeBadRequest, err.Error())
			return
		}
		if errors.Is(err, musicservice.ErrMusicNotFound) {
			response.NotFound(c)
			return
		}
		response.ServerError(c)
		return
	}
	response.Success(c, struct{}{})
}

// DeleteMusic 删除音乐。
// @Summary 删除音乐
// @Description 管理员软删除音乐。
// @Tags 音乐
// @Produce json
// @Param id path int true "音乐 ID"
// @Success 200 {object} response.Response "统一响应；code=0 表示删除成功"
// @Failure 400 {object} response.Response "参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/music/{id} [delete]
func (h *MusicHandler) DeleteMusic(c *gin.Context) {
	id, ok := parseMusicID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteMusic(id); err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, map[string]uint{"id": id})
}

// UploadAudio 上传临时音频文件。
// @Summary 上传临时音频文件
// @Description 管理员上传音乐音频到临时路径，最大 50MB。
// @Tags 音乐
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "音频文件"
// @Success 200 {object} response.Response{data=dto.MusicUploadResp} "统一响应；code=0 表示上传成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/music/uploads/audio [post]
func (h *MusicHandler) UploadAudio(c *gin.Context) {
	claims := jwtpkg.GetClaims(c)
	if claims == nil || claims.UserId <= 0 {
		response.Unauthorized(c)
		return
	}
	data, name, ok := readMusicUploadFile(c, musicservice.MaxMusicAudioBytes)
	if !ok {
		return
	}
	resp, err := h.svc.UploadAudio(c.Request.Context(), musicservice.MusicAudioUploadInput{
		UserID: uint(claims.UserId),
		Name:   name,
		Data:   data,
	})
	writeMusicUploadResponse(c, resp, err)
}

// UploadAlbumCover 上传临时专辑封面。
// @Summary 上传临时专辑封面
// @Description 管理员上传专辑封面到临时路径，最大 10MB。
// @Tags 音乐
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "封面图片"
// @Success 200 {object} response.Response{data=dto.MusicUploadResp} "统一响应；code=0 表示上传成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/music/uploads/album-cover [post]
func (h *MusicHandler) UploadAlbumCover(c *gin.Context) {
	claims := jwtpkg.GetClaims(c)
	if claims == nil || claims.UserId <= 0 {
		response.Unauthorized(c)
		return
	}
	data, name, ok := readMusicUploadFile(c, uploadservice.MaxTempImageBytes)
	if !ok {
		return
	}
	resp, err := h.svc.UploadAlbumCover(c.Request.Context(), musicservice.MusicImageUploadInput{
		UserID: uint(claims.UserId),
		Name:   name,
		Data:   data,
	})
	writeMusicUploadResponse(c, resp, err)
}

// UploadArtistAvatar 上传临时歌手头像。
// @Summary 上传临时歌手头像
// @Description 管理员上传歌手头像到临时路径，最大 10MB。
// @Tags 音乐
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "头像图片"
// @Success 200 {object} response.Response{data=dto.MusicUploadResp} "统一响应；code=0 表示上传成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/music/uploads/artist-avatar [post]
func (h *MusicHandler) UploadArtistAvatar(c *gin.Context) {
	claims := jwtpkg.GetClaims(c)
	if claims == nil || claims.UserId <= 0 {
		response.Unauthorized(c)
		return
	}
	data, name, ok := readMusicUploadFile(c, uploadservice.MaxTempImageBytes)
	if !ok {
		return
	}
	resp, err := h.svc.UploadArtistAvatar(c.Request.Context(), musicservice.MusicImageUploadInput{
		UserID: uint(claims.UserId),
		Name:   name,
		Data:   data,
	})
	writeMusicUploadResponse(c, resp, err)
}

// ListAdminArtists 查询管理端歌手列表。
// @Summary 查询管理端歌手列表
// @Description 管理员查询歌手列表，可按 name 或 name_zh 关键字过滤。
// @Tags 音乐
// @Produce json
// @Param keyword query string false "搜索关键字"
// @Success 200 {object} response.Response{data=dto.MusicArtistListResp} "统一响应；code=0 表示查询成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/music/artists [get]
func (h *MusicHandler) ListAdminArtists(c *gin.Context) {
	resp, err := h.svc.ListArtists(c.Query("keyword"))
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
}

// SaveArtist 新增或修改歌手。
// @Summary 新增或修改歌手
// @Description 管理员新增或修改歌手；PUT 时路径 ID 优先于请求体中的 id。
// @Tags 音乐
// @Accept json
// @Produce json
// @Param id path int false "歌手 ID（仅 PUT）"
// @Param request body dto.MusicArtistSaveReq true "歌手保存请求"
// @Success 200 {object} response.Response{data=dto.MusicArtistResp} "统一响应；code=0 表示保存成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 404 {object} response.Response "歌手不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/music/artists [post]
// @Router /admin/music/artists/{id} [put]
func (h *MusicHandler) SaveArtist(c *gin.Context) {
	var req dto.MusicArtistSaveReq
	if !reqbind.JSON(c, &req) {
		return
	}
	if idStr := c.Param("id"); idStr != "" {
		id, ok := parseMusicID(c)
		if !ok {
			return
		}
		req.ID = id
	}
	claims := jwtpkg.GetClaims(c)
	if claims == nil || claims.UserId <= 0 {
		response.Unauthorized(c)
		return
	}
	resp, err := h.svc.SaveArtist(c.Request.Context(), uint(claims.UserId), req)
	writeMusicArtistResponse(c, resp, err)
}

// DeleteArtist 删除歌手。
// @Summary 删除歌手
// @Description 管理员软删除歌手。
// @Tags 音乐
// @Produce json
// @Param id path int true "歌手 ID"
// @Success 200 {object} response.Response "统一响应；code=0 表示删除成功"
// @Failure 400 {object} response.Response "参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 404 {object} response.Response "歌手不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/music/artists/{id} [delete]
func (h *MusicHandler) DeleteArtist(c *gin.Context) {
	id, ok := parseMusicID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteArtist(id); err != nil {
		if errors.Is(err, musicservice.ErrMusicArtistNotFound) {
			response.NotFound(c)
			return
		}
		response.ServerError(c)
		return
	}
	response.Success(c, struct{}{})
}

// ListAdminAlbums 查询管理端专辑列表。
// @Summary 查询管理端专辑列表
// @Description 管理员查询专辑列表，可按专辑名关键字过滤。
// @Tags 音乐
// @Produce json
// @Param keyword query string false "搜索关键字"
// @Success 200 {object} response.Response{data=dto.MusicAlbumListResp} "统一响应；code=0 表示查询成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/music/albums [get]
func (h *MusicHandler) ListAdminAlbums(c *gin.Context) {
	resp, err := h.svc.ListAlbums(c.Query("keyword"))
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
}

// SaveAlbum 新增或修改专辑。
// @Summary 新增或修改专辑
// @Description 管理员新增或修改专辑；PUT 时路径 ID 优先于请求体中的 id。
// @Tags 音乐
// @Accept json
// @Produce json
// @Param id path int false "专辑 ID（仅 PUT）"
// @Param request body dto.MusicAlbumSaveReq true "专辑保存请求"
// @Success 200 {object} response.Response{data=dto.MusicAlbumResp} "统一响应；code=0 表示保存成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 404 {object} response.Response "歌手或专辑不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/music/albums [post]
// @Router /admin/music/albums/{id} [put]
func (h *MusicHandler) SaveAlbum(c *gin.Context) {
	var req dto.MusicAlbumSaveReq
	if !reqbind.JSON(c, &req) {
		return
	}
	if idStr := c.Param("id"); idStr != "" {
		id, ok := parseMusicID(c)
		if !ok {
			return
		}
		req.ID = id
	}
	claims := jwtpkg.GetClaims(c)
	if claims == nil || claims.UserId <= 0 {
		response.Unauthorized(c)
		return
	}
	resp, err := h.svc.SaveAlbum(c.Request.Context(), uint(claims.UserId), req)
	writeMusicAlbumResponse(c, resp, err)
}

// DeleteAlbum 删除专辑。
// @Summary 删除专辑
// @Description 管理员软删除专辑。
// @Tags 音乐
// @Produce json
// @Param id path int true "专辑 ID"
// @Success 200 {object} response.Response "统一响应；code=0 表示删除成功"
// @Failure 400 {object} response.Response "参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 404 {object} response.Response "专辑不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/music/albums/{id} [delete]
func (h *MusicHandler) DeleteAlbum(c *gin.Context) {
	id, ok := parseMusicID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteAlbum(id); err != nil {
		if errors.Is(err, musicservice.ErrMusicAlbumNotFound) {
			response.NotFound(c)
			return
		}
		response.ServerError(c)
		return
	}
	response.Success(c, struct{}{})
}

func parseMusicID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return 0, false
	}
	return uint(id), true
}

func writeMusicArtistResponse(c *gin.Context, data *dto.MusicArtistResp, err error) {
	if err == nil {
		response.Success(c, data)
		return
	}
	if errors.Is(err, musicservice.ErrMusicArtistNotFound) {
		response.NotFound(c)
		return
	}
	if errors.Is(err, musicservice.ErrMusicAssetInvalid) || errors.Is(err, musicservice.ErrMusicAssetNotFound) {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}
	response.ServerError(c)
}

func writeMusicAlbumResponse(c *gin.Context, data *dto.MusicAlbumResp, err error) {
	if err == nil {
		response.Success(c, data)
		return
	}
	if errors.Is(err, musicservice.ErrMusicArtistNotFound) || errors.Is(err, musicservice.ErrMusicAlbumNotFound) {
		response.NotFound(c)
		return
	}
	if errors.Is(err, musicservice.ErrMusicAssetInvalid) || errors.Is(err, musicservice.ErrMusicAssetNotFound) {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}
	response.ServerError(c)
}

func normalizeMusicAdminListReq(req *dto.MusicAdminListReq) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 {
		req.PageSize = 20
	}
	if req.PageSize > 100 {
		req.PageSize = 100
	}
}

func readMusicUploadFile(c *gin.Context, maxBytes int) ([]byte, string, bool) {
	header, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, response.CodeBadRequest, "缺少上传文件")
		return nil, "", false
	}
	file, err := header.Open()
	if err != nil {
		response.Fail(c, response.CodeBadRequest, "读取上传文件失败")
		return nil, "", false
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil {
		response.Fail(c, response.CodeBadRequest, "读取上传文件失败")
		return nil, "", false
	}
	if len(data) > maxBytes {
		response.Fail(c, response.CodeBadRequest, "文件过大")
		return nil, "", false
	}
	return data, header.Filename, true
}

func writeMusicUploadResponse(c *gin.Context, resp *dto.MusicUploadResp, err error) {
	if err == nil {
		response.Success(c, resp)
		return
	}
	if errors.Is(err, musicservice.ErrMusicUploadInvalid) {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}
	response.ServerError(c)
}
