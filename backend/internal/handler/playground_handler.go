package handler

import (
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// playgroundKeyNamePrefix 集成工作台自动创建的 key 名称前缀。
// 便于用户在密钥列表中识别这些 key 的用途。
const playgroundKeyNamePrefix = "Playground · "

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

// PlaygroundConfigResponse 集成工作台（对话/生图/画布）注入配置。
type PlaygroundConfigResponse struct {
	// GatewayBaseURL 站点网关根地址（不含路径），如 https://api.example.com
	GatewayBaseURL string `json:"gateway_base_url"`
	// PGWBaseURL /pgw 会话代理根地址（如 https://api.example.com/pgw/v1）。
	// 工作台把它当 OpenAI Base URL 使用，密钥由代理在服务端注入。
	PGWBaseURL string `json:"pgw_base_url"`
	// Groups 用户当前可用的分组及其 key。新增分组后此接口实时返回，无需任何配置。
	Groups []PlaygroundGroupConfig `json:"groups"`
}

// GetPlaygroundConfig 返回集成工作台所需的配置：
// 遍历用户可用分组，为每个分组找到（或自动创建）一把绑定该分组的 key。
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

	out := make([]PlaygroundGroupConfig, 0, len(groups))
	for i := range groups {
		g := &groups[i]
		key := keyByGroup[g.ID]
		if key == nil {
			// 该分组下没有 key，自动创建一把（归属当前用户，走现有计费/配额/风控体系）
			created, createErr := h.apiKeyService.Create(ctx, subject.UserID, service.CreateAPIKeyRequest{
				Name:    playgroundKeyNamePrefix + g.Name,
				GroupID: &g.ID,
			})
			if createErr != nil {
				// 单个分组创建失败不阻塞整体配置下发，只跳过该分组
				continue
			}
			key = created
		}

		// /pgw 会话代理短时令牌（签发失败不阻塞配置下发，仅置空）
		pgwToken, tokenErr := h.authService.SignPlaygroundToken(subject.UserID, g.ID)
		if tokenErr != nil {
			pgwToken = ""
		}

		models := make([]string, 0)
		if g.ModelAllowlist.Enabled {
			models = append(models, g.ModelAllowlist.Models...)
		}

		out = append(out, PlaygroundGroupConfig{
			ID:       g.ID,
			Name:     g.Name,
			Platform: g.Platform,
			Key:      key.Key,
			Models:   models,
			PGWToken: pgwToken,
		})
	}

	response.Success(c, PlaygroundConfigResponse{
		GatewayBaseURL: playgroundGatewayBaseURL(c),
		PGWBaseURL:     playgroundGatewayBaseURL(c) + "/pgw/v1",
		Groups:         out,
	})
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
