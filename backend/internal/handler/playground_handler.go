package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// 工作台托管密钥前缀统一使用 service.PlaygroundKeyNamePrefix（阶段 C 起单一来源）。

// PlaygroundGroupConfig 单个分组的集成工作台配置：
// 该用户在此分组下的专用 key（不存在则自动创建）+ 可用模型列表。
type PlaygroundGroupConfig struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	Platform string   `json:"platform"`
	Key      string   `json:"key"`
	Models   []string `json:"models"`
	// PGWToken /pgw 会话代理短时令牌（24h）。使用代理模式时浏览器只持有
	// 该令牌，真实 key 不出服务端；未使用代理模式时忽略此字段。
	PGWToken string `json:"pgw_token"`
}

// PlaygroundAppModelView 用户端下发的模型展示信息（纯展示，不参与计费）。
// MonitorStatus 关联渠道监控的最近检测状态（operational/degraded/failed/error），
// 空串表示未关联监控或监控无检测数据，前端不展示状态标签。
type PlaygroundAppModelView struct {
	ModelID       string `json:"model_id"`
	DisplayName   string `json:"display_name"`
	PriceLabel    string `json:"price_label"`
	UnitHint      string `json:"unit_hint"`
	Description   string `json:"description"`
	SortOrder     int    `json:"sort_order"`
	MonitorStatus string `json:"monitor_status,omitempty"`
}

// PlaygroundAppGroupView 单个应用下某个绑定分组的注入配置：
// 该用户在此分组下的 key + 该分组启用的模型展示清单。
type PlaygroundAppGroupView struct {
	Group  *PlaygroundGroupConfig   `json:"group"`
	Models []PlaygroundAppModelView `json:"models"`
}

// PlaygroundAppView 单个应用的注入配置：绑定的多个分组（多渠道）。
type PlaygroundAppView struct {
	App        string                    `json:"app"`
	InjectMode string                    `json:"inject_mode"`
	Groups     []PlaygroundAppGroupView  `json:"groups"`
}

// PlaygroundConfigResponse 集成工作台（对话/生图/画布）注入配置。
type PlaygroundConfigResponse struct {
	// GatewayBaseURL 站点网关根地址（不含路径），如 https://api.example.com
	GatewayBaseURL string `json:"gateway_base_url"`
	// PGWBaseURL /pgw 会话代理根地址（如 https://api.example.com/pgw/v1）。
	// 工作台把它当 OpenAI Base URL 使用，密钥由代理在服务端注入。
	PGWBaseURL string `json:"pgw_base_url"`
	// LobeTicket lobe 免登录桥票据（typ=lobe，5min 有效）。壳页面用它拼接
	// {lobeBase}/api/sub2api/bridge-login?ticket=... 实现无感登录。
	LobeTicket string `json:"lobe_ticket"`
	// Groups 用户当前可用的分组及其 key。
	// 管理员配置了工作台时，这里只包含被应用绑定的分组；未配置时沿用自动遍历行为。
	Groups []PlaygroundGroupConfig `json:"groups"`
	// Apps 管理员为每个应用配置的注入规则（只含已启用且已绑定分组的应用）。
	// 壳页面据此注入；为空表示管理员尚未配置，此时回退到 Groups 行为。
	Apps []PlaygroundAppView `json:"apps"`
}

// GetPlaygroundConfig 返回集成工作台所需的配置。
//
// 管理员在「工作台注入配置」中配置了应用时：只按配置下发（应用 → 分组 → 模型展示清单），
// 且 key 仍沿用现有的「该分组下没有就自动创建」逻辑。未配置时保持原有自动遍历行为，
// 避免后台还没配置就把客户的工作台搞不可用。
// GET /api/v1/keys/playground-config
func (h *APIKeyHandler) GetPlaygroundConfig(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	ctx := c.Request.Context()

	groups, err := h.apiKeyService.GetAvailableGroups(ctx, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	groupByID := make(map[int64]*service.Group, len(groups))
	for i := range groups {
		groupByID[groups[i].ID] = &groups[i]
	}

	// 管理员配置的应用（只含已启用且已绑定分组的）
	managedApps := map[string]service.PlaygroundRuntimeApp{}
	if h.playgroundConfigService != nil {
		if apps, appErr := h.playgroundConfigService.ListEnabledApps(ctx); appErr == nil && len(apps) > 0 {
			managedApps = apps
		}
	}

	// 拉取用户全部 active key，按 group_id 建索引（List 默认 created_at desc，先命中即最新）
	params := pagination.PaginationParams{
		Page:      1,
		PageSize:  1000,
		SortBy:    "created_at",
		SortOrder: "desc",
	}
	keys, _, err := h.apiKeyService.List(ctx, subject.UserID, params, service.APIKeyListFilters{
		Status: service.StatusActive,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	keyByGroup := make(map[int64]*service.APIKey, len(keys))
	for i := range keys {
		k := &keys[i]
		if k.GroupID == nil {
			continue
		}
		if _, exists := keyByGroup[*k.GroupID]; !exists {
			keyByGroup[*k.GroupID] = k
		}
	}

	apps := make([]PlaygroundAppView, 0, len(managedApps))
	groupOut := make([]PlaygroundGroupConfig, 0, len(groups))
	groupEmitted := make(map[int64]struct{}, len(groups))

	// 管理模式：按管理员配置下发，顺序固定为 chat / image / canvas
	for _, app := range service.PlaygroundSupportedApps {
		runtime, exists := managedApps[app]
		if !exists {
			continue
		}

		appGroups := make([]PlaygroundAppGroupView, 0, len(runtime.Groups))
		for _, rg := range runtime.Groups {
			g := groupByID[rg.GroupID]
			if g == nil {
				// 用户无权使用该分组（或分组已停用）：跳过该分组
				continue
			}
			key, keyErr := h.ensurePlaygroundKey(ctx, subject.UserID, g, keyByGroup)
			if keyErr != nil {
				continue
			}

			modelIDs := make([]string, 0, len(rg.Models))
			modelViews := make([]PlaygroundAppModelView, 0, len(rg.Models))
			for _, m := range rg.Models {
				modelIDs = append(modelIDs, m.ModelID)
				modelViews = append(modelViews, PlaygroundAppModelView{
					ModelID:       m.ModelID,
					DisplayName:   m.DisplayName,
					PriceLabel:    m.PriceLabel,
					UnitHint:      m.UnitHint,
					Description:   m.Description,
					SortOrder:     m.SortOrder,
					MonitorStatus: m.MonitorStatus,
				})
			}
			if len(modelIDs) == 0 {
				// 管理员未标注任何模型时回退到分组白名单，避免工作台无模型可选
				modelIDs = groupAllowlistModels(g)
			}

			groupCfg := h.buildGroupConfig(ctx, subject.UserID, g, key, modelIDs)
			appGroups = append(appGroups, PlaygroundAppGroupView{
				Group:  &groupCfg,
				Models: modelViews,
			})
			if _, dup := groupEmitted[g.ID]; !dup {
				groupEmitted[g.ID] = struct{}{}
				groupOut = append(groupOut, groupCfg)
			}
		}

		if len(appGroups) == 0 {
			continue
		}
		apps = append(apps, PlaygroundAppView{
			App:        app,
			InjectMode: runtime.InjectMode,
			Groups:     appGroups,
		})
	}

	// 回退模式：管理员尚未配置任何应用时，沿用原「遍历用户可用分组」行为
	if len(apps) == 0 {
		for i := range groups {
			g := &groups[i]
			key, keyErr := h.ensurePlaygroundKey(ctx, subject.UserID, g, keyByGroup)
			if keyErr != nil {
				continue
			}
			groupOut = append(groupOut, h.buildGroupConfig(ctx, subject.UserID, g, key, groupAllowlistModels(g)))
		}
	}

	// lobe 免登录票据（签发失败不阻塞配置下发，仅置空）
	lobeTicket, lobeTicketErr := h.authService.SignLobeTicket(subject.UserID)
	if lobeTicketErr != nil {
		lobeTicket = ""
	}

	response.Success(c, PlaygroundConfigResponse{
		GatewayBaseURL: playgroundGatewayBaseURL(c),
		PGWBaseURL:     playgroundGatewayBaseURL(c) + "/pgw/v1",
		LobeTicket:     lobeTicket,
		Groups:         groupOut,
		Apps:           apps,
	})
}

// ensurePlaygroundKey 返回用户在该分组下的 key，不存在则自动创建一把。
func (h *APIKeyHandler) ensurePlaygroundKey(
	ctx context.Context,
	userID int64,
	group *service.Group,
	keyByGroup map[int64]*service.APIKey,
) (*service.APIKey, error) {
	if key, ok := keyByGroup[group.ID]; ok {
		return key, nil
	}
	// 该分组下没有 key，自动创建一把（归属当前用户，走现有计费/配额/风控体系）
	created, err := h.apiKeyService.Create(ctx, userID, service.CreateAPIKeyRequest{
		Name:    service.PlaygroundKeyNamePrefix + group.Name,
		GroupID: &group.ID,
	})
	if err != nil {
		return nil, err
	}
	keyByGroup[group.ID] = created
	return created, nil
}

// buildGroupConfig 组装单个分组的下发配置（含 /pgw 短时令牌）。
func (h *APIKeyHandler) buildGroupConfig(
	ctx context.Context,
	userID int64,
	group *service.Group,
	key *service.APIKey,
	models []string,
) PlaygroundGroupConfig {
	// /pgw 会话代理短时令牌（签发失败不阻塞配置下发，仅置空）
	pgwToken := ""
	if h.authService != nil {
		if token, err := h.authService.SignPlaygroundToken(userID, group.ID); err == nil {
			pgwToken = token
		}
	}
	if models == nil {
		models = []string{}
	}
	return PlaygroundGroupConfig{
		ID:       group.ID,
		Name:     group.Name,
		Platform: group.Platform,
		Key:      key.Key,
		Models:   models,
		PGWToken: pgwToken,
	}
}

// groupAllowlistModels 返回分组白名单中的模型；未启用白名单时返回空列表。
func groupAllowlistModels(group *service.Group) []string {
	models := make([]string, 0)
	if group.ModelAllowlist.Enabled {
		models = append(models, group.ModelAllowlist.Models...)
	}
	return models
}

// playgroundGatewayBaseURL 从请求推导站点对外地址（优先信任反代头），不含路径。
func playgroundGatewayBaseURL(c *gin.Context) string {
	scheme := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = strings.TrimSpace(c.GetHeader("X-Forwarded-Scheme"))
	}
	if scheme == "" && c.Request != nil && c.Request.TLS != nil {
		scheme = "https"
	}
	if scheme == "" {
		scheme = "http"
	}

	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" && c.Request != nil {
		host = c.Request.Host
	}
	if host == "" {
		return ""
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}
