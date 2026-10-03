package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const tokenTTL = 12 * time.Hour

type tokenPayload struct {
	UserID uint   `json:"uid"`
	Role   string `json:"role"`
	Name   string `json:"name"`
	Exp    int64  `json:"exp"`
}

var tokenSecret = []byte("royalty-local-demo-secret-2026")

func b64(b []byte) string           { return base64.RawURLEncoding.EncodeToString(b) }
func db64(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }

func sign(data []byte) []byte {
	m := hmac.New(sha256.New, tokenSecret)
	m.Write(data)
	return m.Sum(nil)
}

func issueToken(u User) string {
	p := tokenPayload{UserID: u.ID, Role: u.Role, Name: u.Name, Exp: time.Now().Add(tokenTTL).Unix()}
	raw, _ := json.Marshal(p)
	body := b64(raw)
	return body + "." + b64(sign([]byte(body)))
}

func parseToken(t string) (tokenPayload, error) {
	var p tokenPayload
	parts := strings.SplitN(t, ".", 2)
	if len(parts) != 2 {
		return p, errors.New("token 格式错误")
	}
	sig, err := db64(parts[1])
	if err != nil || !hmac.Equal(sig, sign([]byte(parts[0]))) {
		return p, errors.New("签名无效")
	}
	raw, err := db64(parts[0])
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, err
	}
	if time.Now().Unix() > p.Exp {
		return p, errors.New("登录已过期")
	}
	return p, nil
}

func hashPassword(password, salt string) string {
	h := sha256.Sum256([]byte(salt + ":" + password))
	return hex.EncodeToString(h[:])
}

func newSalt() string {
	b := make([]byte, 12)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func createUser(db *gorm.DB, email, password, name, role string, authorID *uint) User {
	salt := newSalt()
	u := User{Email: email, Salt: salt, PasswordHash: hashPassword(password, salt), Name: name, Role: role, AuthorID: authorID}
	db.Create(&u)
	return u
}

type authContext struct {
	Payload tokenPayload
	User    *User
}

func authMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
			return
		}
		p, err := parseToken(strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		var u User
		if err := db.First(&u, p.UserID).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
			return
		}
		c.Set("auth", authContext{Payload: p, User: &u})
		c.Next()
	}
}

func mustAuth(c *gin.Context) authContext { return c.MustGet("auth").(authContext) }

func requireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		a := mustAuth(c)
		for _, r := range roles {
			if a.Payload.Role == r {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": fmt.Sprintf("需要角色: %s", strings.Join(roles, "/"))})
	}
}
