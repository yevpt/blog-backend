package moderation

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/multipartlimit"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
	rulemod "github.com/vpt/blog-backend/internal/service/moderationrule"
	"github.com/vpt/blog-backend/pkg/response"
)

// DownloadTemplate 下载导入模板。
// @Summary 下载规则导入模板
// @Description 下载 CSV 或 TXT 格式的导入模板，不含真实敏感词。
// @Tags 审核规则管理
// @Produce text/csv
// @Param format query string true "模板格式：csv 或 txt"
// @Success 200 {file} file
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 400 {object} response.Response
// @Router /admin/moderation/rule-imports/template [get]
func (h *AdminHandler) DownloadTemplate(c *gin.Context) {
	if !h.RulesEnabled() {
		response.NotFound(c)
		return
	}
	format := strings.ToLower(c.Query("format"))
	switch format {
	case "csv":
		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Header("Content-Disposition", `attachment; filename="moderation-rules-template.csv"`)
		c.Status(http.StatusOK)
		_ = rulemod.WriteCSVTemplate(c.Writer)
	case "txt":
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.Header("Content-Disposition", `attachment; filename="moderation-rules-template.txt"`)
		c.Status(http.StatusOK)
		_ = rulemod.WriteTXTTemplate(c.Writer)
	default:
		response.Fail(c, response.CodeBadRequest, "不支持的模板格式")
	}
}

// ExportRules 流式导出当前筛选结果的 CSV。
// @Summary 导出审核规则
// @Description 流式导出当前筛选结果的 UTF-8 BOM CSV，防止公式注入。
// @Tags 审核规则管理
// @Produce text/csv
// @Param pattern query string false "模式搜索"
// @Param search_mode query string false "搜索模式：exact、prefix"
// @Param category query string false "分类"
// @Param rule_type query string false "规则类型"
// @Param risk_level query string false "风险等级"
// @Param effect query string false "规则效果"
// @Param source_id query int false "来源 ID"
// @Param active query bool false "启用状态"
// @Success 200 {file} file
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/rules/export [get]
func (h *AdminHandler) ExportRules(c *gin.Context) {
	if !h.RulesEnabled() {
		response.NotFound(c)
		return
	}
	var req dto.AdminModerationRuleListReq
	if !reqbind.Query(c, &req) {
		return
	}

	query := rulemod.ListQuery{
		ExactID: req.ID, Limit: 100,
		Category: req.Category, RuleType: req.RuleType, RiskLevel: req.RiskLevel,
		Effect: req.Effect, SourceID: req.SourceID, Active: req.Active,
	}
	if req.SearchMode == "exact" {
		query.ExactPattern = req.Pattern
	} else if req.SearchMode == "prefix" {
		query.PatternPrefix = req.Pattern
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="moderation-rules-export.csv"`)
	c.Status(http.StatusOK)
	if err := rulemod.WriteExportHeader(c.Writer); err != nil {
		return
	}

	// 分页拉取规则并流式写入 CSV。
	cursor := uint64(0)
	for {
		query.AfterID = cursor
		page, err := h.ruleSvc.ListRules(c.Request.Context(), query)
		if err != nil {
			return
		}
		exportRules := make([]rulemod.ExportRule, 0, len(page.Rules))
		for _, r := range page.Rules {
			exportRules = append(exportRules, rulemod.ExportRule{
				ID: r.ID, Name: r.Name, RuleType: r.RuleType, Pattern: r.Pattern,
				Category: r.Category, Effect: r.Effect, RiskLevel: r.RiskLevel,
				Priority: r.Priority, SourceID: r.SourceID, Active: r.Active,
			})
		}
		if err := rulemod.WriteExportRows(c.Writer, exportRules); err != nil {
			return
		}
		if !page.HasMore {
			return
		}
		cursor = page.NextCursor
	}
}

// DownloadImportErrors 流式下载导入错误报告。
// @Summary 下载导入错误报告
// @Description 流式下载逐行错误报告 CSV。
// @Tags 审核规则管理
// @Produce text/csv
// @Param id path int true "导入任务 ID"
// @Success 200 {file} file
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/rule-imports/{id}/errors [get]
func (h *AdminHandler) DownloadImportErrors(c *gin.Context) {
	if !h.RulesEnabled() {
		response.NotFound(c)
		return
	}
	id, ok := reqbind.PathUint(c, "id", "导入任务 ID")
	if !ok {
		return
	}

	reader, err := h.ruleSvc.OpenImportErrors(c.Request.Context(), uint64(id))
	if err != nil {
		writeRuleErrorResponse(c, err)
		return
	}
	defer reader.Close()

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="moderation-import-`+strconv.FormatUint(uint64(id), 10)+`-errors.csv"`)
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, reader)
}

// CreateImport 创建导入任务。
// @Summary 创建规则导入任务
// @Description 管理员上传 CSV 或 TXT 文件并创建导入任务。
// @Tags 审核规则管理
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "规则文件"
// @Param format formData string true "格式：csv 或 txt"
// @Param source_name formData string true "来源名称"
// @Param default_category formData string true "缺省分类"
// @Param default_effect formData string true "缺省效果"
// @Param default_risk_level formData string true "缺省风险等级"
// @Param default_priority formData int false "缺省优先级"
// @Success 202 {object} response.Response{data=dto.AdminModerationImportResp}
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 429 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/rule-imports [post]
func (h *AdminHandler) CreateImport(c *gin.Context) {
	if !h.RulesEnabled() {
		response.NotFound(c)
		return
	}
	actorID, ok := requiredRuleActorID(c)
	if !ok {
		return
	}

	if !multipartlimit.Guard(c, multipartlimit.SingleFileMaxBody(h.ruleImportMaxFileBytes)) {
		return
	}

	// 绑定表单参数（非文件字段）。
	var req dto.AdminModerationRuleImportCreateReq
	if !reqbind.Form(c, &req) {
		return
	}

	file, header, err := c.Request.FormFile("file")
	if multipartlimit.RespondParseError(c, err) {
		return
	}
	if err != nil {
		response.Fail(c, response.CodeBadRequest, "文件参数错误")
		return
	}
	defer file.Close()
	if multipartlimit.RejectExcessFileParts(c, 1) {
		return
	}
	if header.Size <= 0 {
		response.Fail(c, response.CodeBadRequest, "文件不能为空")
		return
	}
	if header.Size > int64(h.ruleImportMaxFileBytes) {
		response.Fail(c, response.CodeBadRequest, multipartlimit.ErrBodyTooLarge.Error())
		return
	}

	imp, err := h.ruleSvc.CreateImport(c.Request.Context(), rulemod.CreateImportInput{
		FileName:         header.Filename,
		Format:           req.Format,
		FileSize:         uint64(header.Size),
		Body:             file,
		SourceName:       req.SourceName,
		DefaultCategory:  req.DefaultCategory,
		DefaultEffect:    req.DefaultEffect,
		DefaultRiskLevel: req.DefaultRiskLevel,
		DefaultPriority:  req.DefaultPriority,
		OperatorID:       actorID,
	})
	if err != nil {
		writeRuleErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusAccepted, response.Response{Code: response.CodeOK, Message: "导入任务已创建", Data: importToDTO(imp)})
}
