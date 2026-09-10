package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Playground 代理网关（/pgw）短时令牌。
//
// 浏览器/工作台只持有该令牌（typ=pgw，24h 有效），真实 API Key 由 /pgw
// 反代在服务端注入，不下发到前端。令牌与面板 JWT 共用同一 HS256 密钥，
// 但 typ 隔离：面板 JWT 不能当 pgw 令牌用，反之亦然。
const PlaygroundTokenTTL = 24 * time.Hour

// SignPlaygroundToken 为「用户 + 分组」签发一把 /pgw 短时令牌。
func (s *AuthService) SignPlaygroundToken(userID, groupID int64) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"typ": "pgw",
		"uid": userID,
		"gid": groupID,
		"iat": now.Unix(),
		"exp": now.Add(PlaygroundTokenTTL).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.cfg.JWT.Secret))
	if err != nil {
		return "", fmt.Errorf("sign playground token: %w", err)
	}
	return tokenString, nil
}

// ValidatePlaygroundToken 校验 /pgw 令牌，返回其绑定的用户与分组。
func (s *AuthService) ValidatePlaygroundToken(tokenString string) (userID, groupID int64, err error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(s.cfg.JWT.Secret), nil
	})
	if err != nil {
		return 0, 0, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return 0, 0, errors.New("invalid playground token")
	}
	if typ, _ := claims["typ"].(string); typ != "pgw" {
		return 0, 0, errors.New("invalid playground token type")
	}
	uid, ok := claims["uid"].(float64)
	if !ok {
		return 0, 0, errors.New("invalid playground token uid")
	}
	gid, ok := claims["gid"].(float64)
	if !ok {
		return 0, 0, errors.New("invalid playground token gid")
	}
	return int64(uid), int64(gid), nil
}
