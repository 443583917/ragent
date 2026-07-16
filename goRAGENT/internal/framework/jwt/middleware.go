package jwt

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	jwtv5 "github.com/golang-jwt/jwt/v5"

	"goRAGENT/internal/framework/response"
	"goRAGENT/internal/framework/userctx"
)

type Claims struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Avatar   string `json:"avatar,omitempty"`
	jwtv5.RegisteredClaims
}

// Middleware JWT 鉴权（参数化 tokenName）
func Middleware(tokenName string) gin.HandlerFunc {
	if tokenName == "" { tokenName = "Authorization" }
	return func(c *gin.Context) {
		if c.Request.Method == "OPTIONS" { c.Next(); return }
		token := strings.TrimPrefix(c.GetHeader(tokenName), "Bearer ")
		token = strings.TrimSpace(token)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.Failure(response.CodeNotLogin, "未登录"))
			return
		}
		claims, err := ParseToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.Failure(response.CodeNotLogin, "Token 无效"))
			return
		}
		ctx := userctx.Set(c.Request.Context(), &userctx.LoginUser{UserID: claims.UserID, Username: claims.Username, Role: claims.Role, Avatar: claims.Avatar})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

var jwtSecret = []byte("ragent-go-jwt-secret")

func GenerateToken(uid, uname, role, avatar string) (string, error) {
	t := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, &Claims{UserID: uid, Username: uname, Role: role, Avatar: avatar})
	return t.SignedString(jwtSecret)
}

func ParseToken(s string) (*Claims, error) {
	t, err := jwtv5.ParseWithClaims(s, &Claims{}, func(t *jwtv5.Token) (any, error) { return jwtSecret, nil })
	if err != nil { return nil, err }
	if c, ok := t.Claims.(*Claims); ok && t.Valid { return c, nil }
	return nil, fmt.Errorf("invalid token")
}
