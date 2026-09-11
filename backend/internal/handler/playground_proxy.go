package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// =============================================================================
// /pgw 会话代理网关（fork 二开）
//
// 目标：把工作台请求里的真实 API Key 藏在服务端。
//   浏览器只持有 24h 短时 pgw 令牌（typ=pgw，uid+gid）→ /pgw/v1/*
//   本处理器校验令牌 → 找到（或创建）该用户在该分组下的 playground key
//   → 反代到本机 /v1 网关并注入 Authorization: Bearer <真实key>
//
// 令牌泄露的影响面：24h 内他人可消耗该用户的配额（与直接泄露 key 相同），
// 但不会被长期持久化在第三方应用的 localStorage/URL 里。
// =============================================================================

const pgwCacheTTL = 5 * time.Minute

type pgwKeyCacheEntry struct {
	key *service.APIKey
	exp time.Time
}

// pgwKeyCache进程内缓存 uid:gid → key，避免每个网关请求都查一次库。
// key 被删除/禁用时网关自身会拒绝请求，缓存最多带来 5 分钟的滞后。
type pgwKeyCache struct {
	mu      sync.Mutex
	entries map[string]pgwKeyCacheEntry
}

var pgwKeyCacheInstance = &pgwKeyCache{entries: make(map[string]pgwKeyCacheEntry)}

func (c *pgwKeyCache) get(userID, groupID int64) (*service.APIKey, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[pgwCacheKey(userID, groupID)]
	if !ok || time.Now().After(entry.exp) {
		return nil, false
	}
	return entry.key, true
}

func (c *pgwKeyCache) set(userID, groupID int64, key *service.APIKey) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[pgwCacheKey(userID, groupID)] = pgwKeyCacheEntry{key: key, exp: time.Now().Add(pgwCacheTTL)}
}

func pgwCacheKey(userID, groupID int64) string {
	return fmt.Sprintf("%d:%d", userID, groupID)
}

// resolvePlaygroundKey 找到（或创建）用户在指定分组下的 playground key。
func (h *APIKeyHandler) resolvePlaygroundKey(ctx context.Context, userID, groupID int64, groupName string) (*service.APIKey, error) {
	params := pagination.PaginationParams{
		Page:      1,
		PageSize:  1000,
		SortBy:    "created_at",
		SortOrder: "desc",
	}
	keys, _, err := h.apiKeyService.List(ctx, userID, params, service.APIKeyListFilters{
		Status: service.StatusActive,
	})
	if err != nil {
		return nil, err
	}
	for i := range keys {
		k := &keys[i]
		if k.GroupID != nil && *k.GroupID == groupID {
			return k, nil
		}
	}
	name := groupName
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("Group #%d", groupID)
	}
	return h.apiKeyService.Create(ctx, userID, service.CreateAPIKeyRequest{
		Name:    service.PlaygroundKeyNamePrefix + name,
		GroupID: &groupID,
	})
}

// PGWCORS 为 /pgw 提供跨域支持（工作台部署在同根域名的其他子域名上）。
// 认证走 Authorization 头而非 Cookie，反射 Origin 不会引入 CSRF 面。
func (h *APIKeyHandler) PGWCORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, x-api-key, x-goog-api-key, anthropic-version, anthropic-beta")
			c.Header("Access-Control-Max-Age", "86400")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// PGWTokenAuth 校验 pgw 短时令牌并解析出绑定的真实 key。
func (h *APIKeyHandler) PGWTokenAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractBearerToken(c)
		if token == "" {
			response.Unauthorized(c, "Playground proxy token required")
			return
		}
		userID, groupID, err := h.authService.ValidatePlaygroundToken(token)
		if err != nil {
			response.Unauthorized(c, "Invalid or expired playground proxy token")
			return
		}

		var apiKey *service.APIKey
		if cached, ok := pgwKeyCacheInstance.get(userID, groupID); ok {
			apiKey = cached
		} else {
			apiKey, err = h.resolvePlaygroundKey(c.Request.Context(), userID, groupID, "")
			if err != nil {
				response.ErrorFrom(c, err)
				return
			}
			pgwKeyCacheInstance.set(userID, groupID, apiKey)
		}

		if apiKey.Status != service.StatusActive {
			response.Unauthorized(c, "Playground key is inactive")
			return
		}
		c.Set("pgw_api_key", apiKey.Key)
		c.Next()
	}
}

func extractBearerToken(c *gin.Context) string {
	if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	if key := c.GetHeader("x-goog-api-key"); key != "" {
		return strings.TrimSpace(key)
	}
	if key := c.GetHeader("x-api-key"); key != "" {
		return strings.TrimSpace(key)
	}
	return ""
}

// PGWProxy 反代到本机 /v1 网关，服务端注入真实 key。
func (h *APIKeyHandler) PGWProxy() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetString("pgw_api_key")
		if apiKey == "" {
			response.Unauthorized(c, "Playground proxy key missing")
			return
		}

		base := playgroundGatewayBaseURL(c)
		if base == "" {
			response.Error(c, http.StatusBadGateway, "Cannot determine upstream address")
			return
		}
		target, err := url.Parse(base)
		if err != nil {
			response.Error(c, http.StatusBadGateway, "Invalid upstream address")
			return
		}

		// /pgw/v1/chat/completions → /v1/chat/completions
		upstreamPath := c.Param("path")
		if !strings.HasPrefix(upstreamPath, "/") {
			upstreamPath = "/" + upstreamPath
		}

		proxy := httputil.NewSingleHostReverseProxy(target)
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			req.URL.Path = upstreamPath
			req.Header.Set("Authorization", "Bearer "+apiKey)
			req.Header.Del("x-api-key")
			req.Header.Del("x-goog-api-key")
		}
		// SSE/流式响应立即转发
		proxy.FlushInterval = -1
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("playground proxy upstream error"))
		}
		proxy.ServeHTTP(c.Writer, c.Request)
	}
}
