package captcha

import (
	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
	captchaservice "github.com/vpt/blog-backend/internal/service/captcha"
	"github.com/vpt/blog-backend/pkg/response"
)

// CaptchaHandler 图形验证码模块 handler，对应 /captcha 路由。
type CaptchaHandler struct {
	svc captchaservice.Service
}

func NewCaptchaHandler(svc captchaservice.Service) *CaptchaHandler {
	return &CaptchaHandler{svc: svc}
}

// GenerateRegistrationChallenge 生成注册场景的 GoCaptcha 滑块挑战。
// @Summary 生成注册图形验证码
// @Description 返回 GoCaptcha 滑块主图、滑块图和挑战 ID；后端只保存答案，不向前端暴露目标 X 坐标。
// @Tags 图形验证码
// @Accept json
// @Produce json
// @Success 200 {object} response.Response{data=dto.CaptchaChallengeResp} "生成成功"
// @Failure 500 {object} response.Response "生成失败"
// @Router /captcha/register/challenge [post]
func (h *CaptchaHandler) GenerateRegistrationChallenge(c *gin.Context) {
	challenge, err := h.svc.GenerateRegistrationChallenge()
	if err != nil {
		response.ServerError(c)
		return
	}

	response.Success(c, challenge)
}

// VerifyRegistrationChallenge 校验注册场景的 GoCaptcha 滑块坐标。
// @Summary 校验注册图形验证码
// @Description 校验通过后返回短期一次性 captcha_token，用于 /auth/send-code。
// @Tags 图形验证码
// @Accept json
// @Produce json
// @Param request body dto.CaptchaVerifyReq true "图形验证码校验请求"
// @Success 200 {object} response.Response{data=dto.CaptchaVerifyResp} "统一响应；code=0 表示校验成功，code=400 表示参数错误或验证码错误"
// @Router /captcha/register/verify [post]
func (h *CaptchaHandler) VerifyRegistrationChallenge(c *gin.Context) {
	var req dto.CaptchaVerifyReq
	if !reqbind.JSON(c, &req) {
		return
	}

	result, err := h.svc.VerifyRegistrationChallenge(&req, c.ClientIP())
	if err != nil {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}

	response.Success(c, result)
}
