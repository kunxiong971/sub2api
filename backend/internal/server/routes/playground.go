package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterPlaygroundProxyRoutes 注册 /pgw 会话代理网关（fork 二开）。
//
// 工作台把 OpenAI Base URL指向 {站点}/pgw/v1，密钥用 playground-config
// 下发的短时令牌（typ=pgw）。代理在校验令牌后，向本机 /v1 网关注入该用户
// 在令牌绑定分组下的真实 key——真实 key 不出现在任何前端存储中。
func RegisterPlaygroundProxyRoutes(r *gin.Engine, h *handler.Handlers) {
	pgw := r.Group("/pgw")
	pgw.Use(h.APIKey.PGWCORS())
	pgw.Use(h.APIKey.PGWTokenAuth())
	{
		// 兼容全部网关协议形态：/pgw/v1/*、/pgw/v1beta/*、/pgw/responses 等
		pgw.Any("/*path", h.APIKey.PGWProxy())
	}
}
