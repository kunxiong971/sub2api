package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// PlaygroundHandler 管理端「工作台注入配置」接口。
//
// 这里配置的都是策略规则（应用 → 多个分组 → 每组模型展示清单），不包含任何
// 真实密钥；密钥在运行时由用户端接口按其在该分组下的 key（或 pgw 短令牌）下发。
type PlaygroundHandler struct {
	playgroundConfigService *service.PlaygroundConfigService
}

// NewPlaygroundHandler 创建工作台注入配置 handler。
func NewPlaygroundHandler(playgroundConfigService *service.PlaygroundConfigService) *PlaygroundHandler {
	return &PlaygroundHandler{playgroundConfigService: playgroundConfigService}
}

// playgroundAppModelInput 单条模型展示配置。
type playgroundAppModelInput struct {
	ModelID     string `json:"model_id"`
	DisplayName string `json:"display_name"`
	PriceLabel  string `json:"price_label"`
	UnitHint    string `json:"unit_hint"`
	Description string `json:"description"`
	Enabled     *bool  `json:"enabled"`
	SortOrder   int    `json:"sort_order"`
}

// playgroundBindingInput 单个分组绑定的保存输入。
type playgroundBindingInput struct {
	GroupID int64                    `json:"group_id"`
	Enabled bool                     `json:"enabled"`
	Models  []playgroundAppModelInput `json:"models"`
}

// resolvePlaygroundApp 校验并归一化 URL 中的应用标识。
func resolvePlaygroundApp(c *gin.Context) (string, bool) {
	app := service.NormalizePlaygroundApp(c.Param("app"))
	if app == "" {
		response.BadRequest(c, "Invalid playground app")
		return "", false
	}
	return app, true
}

// ListPlaygroundConfigs 返回三个应用的配置总览（每个应用含绑定的多个分组）。
// GET /api/v1/admin/playground/configs
func (h *PlaygroundHandler) ListPlaygroundConfigs(c *gin.Context) {
	bundles, err := h.playgroundConfigService.ListApps(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"apps": bundles})
}

// UpdatePlaygroundConfig 全量保存某应用的绑定分组与模型清单。
// PUT /api/v1/admin/playground/configs/:app
func (h *PlaygroundHandler) UpdatePlaygroundConfig(c *gin.Context) {
	app, ok := resolvePlaygroundApp(c)
	if !ok {
		return
	}
	var req struct {
		Bindings []playgroundBindingInput `json:"bindings"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}

	bindings := make([]service.PlaygroundBindingInput, 0, len(req.Bindings))
	for _, b := range req.Bindings {
		models := make([]service.PlaygroundAppModel, 0, len(b.Models))
		for _, m := range b.Models {
			enabled := true
			if m.Enabled != nil {
				enabled = *m.Enabled
			}
			models = append(models, service.PlaygroundAppModel{
				ModelID:     m.ModelID,
				DisplayName: m.DisplayName,
				PriceLabel:  m.PriceLabel,
				UnitHint:    m.UnitHint,
				Description: m.Description,
				Enabled:     enabled,
				SortOrder:   m.SortOrder,
			})
		}
		bindings = append(bindings, service.PlaygroundBindingInput{
			GroupID: b.GroupID,
			Enabled: b.Enabled,
			Models:  models,
		})
	}

	if err := h.playgroundConfigService.SaveApp(c.Request.Context(), app, bindings); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// 返回保存后的该应用总览
	bundles, err := h.playgroundConfigService.ListApps(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	for i := range bundles {
		if bundles[i].App == app {
			response.Success(c, gin.H{"app": bundles[i]})
			return
		}
	}
	response.Success(c, gin.H{"app": nil})
}

// GetPlaygroundModelCandidates 拉取某分组可配置的模型清单。
// GET /api/v1/admin/playground/configs/:app/models/candidates?group_id=3
func (h *PlaygroundHandler) GetPlaygroundModelCandidates(c *gin.Context) {
	if _, ok := resolvePlaygroundApp(c); !ok {
		return
	}
	var groupID int64
	if raw := c.Query("group_id"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			groupID = parsed
		}
	}
	models, err := h.playgroundConfigService.GetModelCandidates(c.Request.Context(), groupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"models": models})
}
