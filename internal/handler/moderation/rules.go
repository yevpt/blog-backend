package moderation

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
	repoMod "github.com/vpt/blog-backend/internal/repository/moderationrule"
	rulemod "github.com/vpt/blog-backend/internal/service/moderationrule"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

// ListRules 查询规则列表。
// @Summary 查询审核规则列表
// @Description 管理员按游标分页、筛选条件查询规则。
// @Tags 审核规则管理
// @Produce json
// @Param cursor query int false "游标，上一页最后一条规则 ID"
// @Param limit query int false "每页数量，默认 20，最大 100"
// @Param id query int false "精确规则 ID"
// @Param pattern query string false "模式搜索"
// @Param search_mode query string false "搜索模式：exact、prefix"
// @Param category query string false "分类"
// @Param rule_type query string false "规则类型"
// @Param risk_level query string false "风险等级"
// @Param effect query string false "规则效果"
// @Param source_id query int false "来源 ID"
// @Param active query bool false "启用状态"
// @Success 200 {object} response.Response{data=dto.AdminModerationRulePageResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/rules [get]
func (h *AdminHandler) ListRules(c *gin.Context) {
	if !h.RulesEnabled() {
		response.NotFound(c)
		return
	}
	var req dto.AdminModerationRuleListReq
	if !reqbind.Query(c, &req) {
		return
	}

	query := rulemod.ListQuery{
		AfterID: req.Cursor, ExactID: req.ID, Limit: req.Limit,
		Category: req.Category, RuleType: req.RuleType, RiskLevel: req.RiskLevel,
		Effect: req.Effect, SourceID: req.SourceID, Active: req.Active,
	}
	if req.SearchMode == "exact" {
		query.ExactPattern = req.Pattern
	} else if req.SearchMode == "prefix" {
		query.PatternPrefix = req.Pattern
	}

	page, err := h.ruleSvc.ListRules(c.Request.Context(), query)
	if err != nil {
		writeRuleErrorResponse(c, err)
		return
	}
	response.Success(c, rulePageToDTO(page))
}

// CreateRule 新增规则。
// @Summary 新增审核规则
// @Description 创建单条规则并触发候选构建发布。
// @Tags 审核规则管理
// @Accept json
// @Produce json
// @Param body body dto.AdminModerationRuleSaveReq true "规则内容"
// @Success 202 {object} response.Response{data=dto.AdminModerationRuleJobResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 409 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/rules [post]
func (h *AdminHandler) CreateRule(c *gin.Context) {
	if !h.RulesEnabled() {
		response.NotFound(c)
		return
	}
	actorID, ok := requiredRuleActorID(c)
	if !ok {
		return
	}
	var req dto.AdminModerationRuleSaveReq
	if !reqbind.JSON(c, &req) {
		return
	}

	job, err := h.ruleSvc.CreateRule(c.Request.Context(), rulemod.CreateRuleCommand{
		ExpectedRulesetID: req.ExpectedRulesetVersion,
		ActorID:           actorID,
		Rule: rulemod.RuleInput{
			Name: req.Name, RuleType: req.RuleType, Pattern: req.Pattern,
			Category: req.Category, Effect: req.Effect, RiskLevel: req.RiskLevel,
			Priority: req.Priority, SourceID: req.SourceID,
		},
	})
	writeRuleJobResponse(c, job, err, http.StatusAccepted)
}

// ReplaceRule 创建替代规则。
// @Summary 替代审核规则
// @Description 创建新规则替代旧规则，旧规则在发布后停用。
// @Tags 审核规则管理
// @Accept json
// @Produce json
// @Param id path int true "被替代的规则 ID"
// @Param body body dto.AdminModerationRuleSaveReq true "新规则内容"
// @Success 202 {object} response.Response{data=dto.AdminModerationRuleJobResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 409 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/rules/{id} [patch]
func (h *AdminHandler) ReplaceRule(c *gin.Context) {
	if !h.RulesEnabled() {
		response.NotFound(c)
		return
	}
	actorID, ok := requiredRuleActorID(c)
	if !ok {
		return
	}
	ruleID, ok := reqbind.PathUint(c, "id", "规则 ID")
	if !ok {
		return
	}
	var req dto.AdminModerationRuleSaveReq
	if !reqbind.JSON(c, &req) {
		return
	}

	job, err := h.ruleSvc.ReplaceRule(c.Request.Context(), rulemod.ReplaceRuleCommand{
		RuleID:            uint64(ruleID),
		ExpectedRulesetID: req.ExpectedRulesetVersion,
		ActorID:           actorID,
		Rule: rulemod.RuleInput{
			Name: req.Name, RuleType: req.RuleType, Pattern: req.Pattern,
			Category: req.Category, Effect: req.Effect, RiskLevel: req.RiskLevel,
			Priority: req.Priority, SourceID: req.SourceID,
		},
	})
	writeRuleJobResponse(c, job, err, http.StatusAccepted)
}

// BatchStatus 批量启停规则。
// @Summary 批量启停审核规则
// @Description 最多 1000 个 ID，批量启用或停用并发布候选版本。
// @Tags 审核规则管理
// @Accept json
// @Produce json
// @Param body body dto.AdminModerationRuleBatchStatusReq true "批量操作请求"
// @Success 202 {object} response.Response{data=dto.AdminModerationRuleJobResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 409 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/rules/batch-status [post]
func (h *AdminHandler) BatchStatus(c *gin.Context) {
	if !h.RulesEnabled() {
		response.NotFound(c)
		return
	}
	actorID, ok := requiredRuleActorID(c)
	if !ok {
		return
	}
	var req dto.AdminModerationRuleBatchStatusReq
	if !reqbind.JSON(c, &req) {
		return
	}

	job, err := h.ruleSvc.BatchStatus(c.Request.Context(), rulemod.BatchStatusCommand{
		ExpectedRulesetID: req.ExpectedRulesetVersion,
		ActorID:           actorID,
		RuleIDs:           req.RuleIDs,
		Active:            req.Active,
	})
	writeRuleJobResponse(c, job, err, http.StatusAccepted)
}

// TestRuleText 文本试跑。
// @Summary 审核规则文本试跑
// @Description 使用当前或候选规则集执行文本试跑，返回命中详情。
// @Tags 审核规则管理
// @Accept json
// @Produce json
// @Param body body dto.AdminModerationRuleTestReq true "试跑请求"
// @Success 200 {object} response.Response{data=dto.AdminModerationRuleTestResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/rules/test [post]
func (h *AdminHandler) TestRuleText(c *gin.Context) {
	if !h.RulesEnabled() {
		response.NotFound(c)
		return
	}
	actorID, ok := requiredRuleActorID(c)
	if !ok {
		return
	}
	var req dto.AdminModerationRuleTestReq
	if !reqbind.JSON(c, &req) {
		return
	}

	result, err := h.ruleSvc.TestText(c.Request.Context(), rulemod.TestTextCommand{
		Text: req.Text, RulesetID: req.RulesetID, ActorID: actorID,
	})
	if err != nil {
		writeRuleErrorResponse(c, err)
		return
	}
	response.Success(c, ruleTestResultToDTO(result))
}

// GetRuleStatus 查询规则集状态。
// @Summary 查询规则集状态
// @Description 返回当前版本、候选任务、规则数和索引统计。
// @Tags 审核规则管理
// @Produce json
// @Success 200 {object} response.Response{data=dto.AdminModerationRuleStatusResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/rules/status [get]
func (h *AdminHandler) GetRuleStatus(c *gin.Context) {
	if !h.RulesEnabled() {
		response.NotFound(c)
		return
	}
	status, err := h.ruleSvc.Status(c.Request.Context())
	if err != nil {
		writeRuleErrorResponse(c, err)
		return
	}
	response.Success(c, ruleStatusToDTO(status))
}

// GetRuleMetadata 查询规则目录元数据。
// @Summary 查询规则目录元数据
// @Description 返回分类、类型、风险、效果和来源目录。
// @Tags 审核规则管理
// @Produce json
// @Success 200 {object} response.Response{data=dto.AdminModerationRuleMetadataResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/rules/metadata [get]
func (h *AdminHandler) GetRuleMetadata(c *gin.Context) {
	if !h.RulesEnabled() {
		response.NotFound(c)
		return
	}
	meta, err := h.ruleSvc.Metadata(c.Request.Context())
	if err != nil {
		writeRuleErrorResponse(c, err)
		return
	}
	response.Success(c, ruleMetadataToDTO(meta))
}

// ListImports 查询导入历史。
// @Summary 查询导入历史
// @Description 按游标分页查询导入任务历史。
// @Tags 审核规则管理
// @Produce json
// @Param cursor query int false "游标"
// @Param limit query int false "每页数量"
// @Success 200 {object} response.Response{data=dto.AdminModerationImportPageResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/rule-imports [get]
func (h *AdminHandler) ListImports(c *gin.Context) {
	if !h.RulesEnabled() {
		response.NotFound(c)
		return
	}
	var req struct {
		Cursor uint64 `form:"cursor"`
		Limit  int    `form:"limit" binding:"omitempty,min=1,max=100"`
	}
	if !reqbind.Query(c, &req) {
		return
	}

	page, err := h.ruleSvc.ListImports(c.Request.Context(), req.Cursor, req.Limit)
	if err != nil {
		writeRuleErrorResponse(c, err)
		return
	}
	response.Success(c, importPageToDTO(page))
}

// GetImport 查询导入详情。
// @Summary 查询导入任务详情
// @Description 返回导入任务进度、统计和候选版本。
// @Tags 审核规则管理
// @Produce json
// @Param id path int true "导入任务 ID"
// @Success 200 {object} response.Response{data=dto.AdminModerationImportResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/rule-imports/{id} [get]
func (h *AdminHandler) GetImport(c *gin.Context) {
	if !h.RulesEnabled() {
		response.NotFound(c)
		return
	}
	id, ok := reqbind.PathUint(c, "id", "导入任务 ID")
	if !ok {
		return
	}
	imp, err := h.ruleSvc.GetImport(c.Request.Context(), uint64(id))
	if err != nil {
		writeRuleErrorResponse(c, err)
		return
	}
	response.Success(c, importToDTO(imp))
}

// PublishImport 发布导入候选。
// @Summary 发布导入候选规则集
// @Description 确认发布 ready 候选，需带期望版本号。
// @Tags 审核规则管理
// @Accept json
// @Produce json
// @Param id path int true "导入任务 ID"
// @Param body body dto.AdminModerationRuleImportPublishReq true "发布请求"
// @Success 200 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 409 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/rule-imports/{id}/publish [post]
func (h *AdminHandler) PublishImport(c *gin.Context) {
	if !h.RulesEnabled() {
		response.NotFound(c)
		return
	}
	actorID, ok := requiredRuleActorID(c)
	if !ok {
		return
	}
	id, ok := reqbind.PathUint(c, "id", "导入任务 ID")
	if !ok {
		return
	}
	var req dto.AdminModerationRuleImportPublishReq
	if !reqbind.JSON(c, &req) {
		return
	}

	imp, err := h.ruleSvc.GetImport(c.Request.Context(), uint64(id))
	if err != nil {
		writeRuleErrorResponse(c, err)
		return
	}
	if imp.RulesetID == nil {
		response.Fail(c, response.CodeBadRequest, "导入任务没有关联的候选规则集")
		return
	}

	if err := h.ruleSvc.PublishCandidate(c.Request.Context(), *imp.RulesetID, req.ExpectedRulesetVersion, actorID); err != nil {
		writeRuleErrorResponse(c, err)
		return
	}
	response.Success(c, nil)
}

// CancelImport 取消导入任务。
// @Summary 取消导入任务
// @Description 取消尚未发布的导入任务。
// @Tags 审核规则管理
// @Produce json
// @Param id path int true "导入任务 ID"
// @Success 200 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/rule-imports/{id} [delete]
func (h *AdminHandler) CancelImport(c *gin.Context) {
	if !h.RulesEnabled() {
		response.NotFound(c)
		return
	}
	actorID, ok := requiredRuleActorID(c)
	if !ok {
		return
	}
	id, ok := reqbind.PathUint(c, "id", "导入任务 ID")
	if !ok {
		return
	}

	if err := h.ruleSvc.CancelImport(c.Request.Context(), uint64(id), actorID); err != nil {
		writeRuleErrorResponse(c, err)
		return
	}
	response.Success(c, nil)
}

func requiredRuleActorID(c *gin.Context) (uint64, bool) {
	claims := jwtpkg.GetClaims(c)
	if claims == nil || claims.UserId <= 0 {
		response.Unauthorized(c)
		return 0, false
	}
	return uint64(claims.UserId), true
}

func writeRuleErrorResponse(c *gin.Context, err error) {
	switch {
	case errors.Is(err, rulemod.ErrRulesetConflict):
		response.Conflict(c, response.CodeModerationRulesetConflict, "规则集版本已更新，请刷新后重试")
	case errors.Is(err, rulemod.ErrRuleLimit), errors.Is(err, rulemod.ErrBatchLimit):
		response.Fail(c, response.CodeBadRequest, "规则数量超限")
	case errors.Is(err, rulemod.ErrIndexMemoryLimit):
		response.Fail(c, response.CodeBadRequest, "索引内存超限")
	case errors.Is(err, rulemod.ErrImportInvalid):
		response.Fail(c, response.CodeBadRequest, "导入文件无效")
	case errors.Is(err, rulemod.ErrCandidateNotFound), errors.Is(err, rulemod.ErrRuleNotFound), errors.Is(err, rulemod.ErrImportReportNotFound):
		response.NotFound(c)
	case errors.Is(err, rulemod.ErrCandidateNotReady):
		response.Fail(c, response.CodeBadRequest, "候选规则集尚未就绪")
	case errors.Is(err, rulemod.ErrInvalidRule), errors.Is(err, rulemod.ErrDuplicateRule), errors.Is(err, rulemod.ErrEmptyRuleset):
		response.Fail(c, response.CodeBadRequest, err.Error())
	default:
		response.ServerError(c)
	}
}

func writeRuleJobResponse(c *gin.Context, job rulemod.Job, err error, successCode int) {
	if err != nil {
		writeRuleErrorResponse(c, err)
		return
	}
	resp := dto.AdminModerationRuleJobResp{
		RulesetID: job.RulesetID, BaseRulesetID: job.BaseRulesetID, Status: job.Status,
	}
	if successCode == http.StatusAccepted {
		c.JSON(http.StatusAccepted, response.Response{Code: response.CodeOK, Message: "ok", Data: resp})
	} else {
		response.Success(c, resp)
	}
}

func rulePageToDTO(page repoMod.RulePage) dto.AdminModerationRulePageResp {
	list := make([]dto.AdminModerationRuleResp, 0, len(page.Rules))
	for _, r := range page.Rules {
		list = append(list, dto.AdminModerationRuleResp{
			ID: r.ID, Name: r.Name, RuleType: r.RuleType, Pattern: r.Pattern,
			Category: r.Category, Effect: r.Effect, RiskLevel: r.RiskLevel,
			Priority: r.Priority, SourceID: r.SourceID,
			ActivatedRulesetID: r.ActivatedRulesetID, DeactivatedRulesetID: r.DeactivatedRulesetID,
			ReplacesRuleID: r.ReplacesRuleID, Active: r.Active,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}
	return dto.AdminModerationRulePageResp{
		List: list, NextCursor: page.NextCursor, HasMore: page.HasMore,
	}
}

func ruleTestResultToDTO(result rulemod.TestResult) dto.AdminModerationRuleTestResp {
	hits := make([]dto.AdminModerationRuleTestHitResp, 0, len(result.Hits))
	for _, hit := range result.Hits {
		hits = append(hits, dto.AdminModerationRuleTestHitResp{
			RuleID: hit.RuleID, RuleType: hit.RuleType, Pattern: hit.Pattern,
			Category: hit.Category, RiskLevel: hit.RiskLevel, Effect: hit.Effect, Excerpt: hit.Excerpt,
		})
	}
	return dto.AdminModerationRuleTestResp{
		Risk: result.Risk, RulesetID: result.RulesetID,
		RuleIDs: result.RuleIDs, SuppressedIDs: result.SuppressedIDs,
		Truncated: result.Truncated, Hits: hits,
	}
}

func ruleStatusToDTO(status rulemod.Status) dto.AdminModerationRuleStatusResp {
	resp := dto.AdminModerationRuleStatusResp{
		CurrentRulesetID: status.CurrentRulesetID,
		RuleCount:        status.RuleCount, KeywordCount: status.KeywordCount,
		RegexpCount: status.RegexpCount, CompositeCount: status.CompositeCount,
		IndexBytes: status.IndexBytes, BuildPeakBytes: status.BuildPeakBytes,
		BuildDurationMS: status.BuildDurationMS, UpdatedAt: status.UpdatedAt,
	}
	if status.Candidate != nil {
		resp.Candidate = &dto.AdminModerationCandidateStatusResp{
			RulesetID: status.Candidate.RulesetID, Status: status.Candidate.Status,
			BaseRulesetID: status.Candidate.BaseRulesetID, RuleCount: status.Candidate.RuleCount,
			IndexBytes: status.Candidate.IndexBytes, BuildPeakBytes: status.Candidate.BuildPeakBytes,
			FailureCode: status.Candidate.FailureCode,
			CreatedAt:   status.Candidate.CreatedAt, UpdatedAt: status.Candidate.UpdatedAt,
		}
	}
	return resp
}

func ruleMetadataToDTO(meta rulemod.Metadata) dto.AdminModerationRuleMetadataResp {
	categories := make([]dto.AdminModerationCategoryEntry, 0, len(meta.Categories))
	for _, cat := range meta.Categories {
		categories = append(categories, dto.AdminModerationCategoryEntry{Key: cat.Key, Name: cat.Name})
	}
	sources := make([]dto.AdminModerationSourceEntry, 0, len(meta.Sources))
	for _, src := range meta.Sources {
		sources = append(sources, dto.AdminModerationSourceEntry{ID: src.ID, Name: src.Name})
	}
	return dto.AdminModerationRuleMetadataResp{
		Categories: categories, RiskLevels: meta.RiskLevels,
		Effects: meta.Effects, RuleTypes: meta.RuleTypes, Sources: sources,
	}
}

func importToDTO(imp repoMod.ImportRecord) dto.AdminModerationImportResp {
	return dto.AdminModerationImportResp{
		ID: imp.ID, FileName: imp.FileName, Format: imp.Format, FileSize: imp.FileSize,
		SourceID: imp.SourceID, DefaultCategory: imp.DefaultCategory,
		DefaultEffect: imp.DefaultEffect, DefaultRiskLevel: imp.DefaultRiskLevel,
		DefaultPriority: imp.DefaultPriority, ValidationStatus: imp.ValidationStatus,
		TotalRows: imp.TotalRows, ValidRows: imp.ValidRows, DuplicateRows: imp.DuplicateRows,
		ErrorRows: imp.ErrorRows, ErrorObjectKey: imp.ErrorObjectKey, RulesetID: imp.RulesetID,
		OperatorID: imp.OperatorID, CreatedAt: imp.CreatedAt, UpdatedAt: imp.UpdatedAt,
	}
}

func importPageToDTO(page repoMod.ImportPage) dto.AdminModerationImportPageResp {
	list := make([]dto.AdminModerationImportResp, 0, len(page.Imports))
	for _, imp := range page.Imports {
		list = append(list, importToDTO(imp))
	}
	return dto.AdminModerationImportPageResp{
		List: list, NextCursor: page.NextCursor, HasMore: page.HasMore,
	}
}

var _ = strconv.FormatUint
