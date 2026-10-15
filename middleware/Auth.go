package middleware

import (
	"log"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// JwtSecret อ่านจาก JWT_SECRET ใน .env (ใช้ร่วมกับ handlers.Login ตอนสร้าง token)
var JwtSecret = loadJwtSecret()

func loadJwtSecret() []byte {
	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		return []byte(secret)
	}
	log.Println("⚠️ ไม่ได้ตั้งค่า JWT_SECRET ใช้ค่า default (ห้ามใช้ใน production)")
	return []byte("secret-key")
}

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// รับ Authorization header จาก Angular's authInterceptor
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(401, gin.H{"error": "ไม่มี token"})
			c.Abort() // หยุดไม่ให้ไปต่อ
			return
		}

		// ตัด "Bearer " ออก
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		// ตรวจ token (รับเฉพาะ HS256 กันการปลอม token ด้วย algorithm อื่น)
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
			return JwtSecret, nil
		}, jwt.WithValidMethods([]string{"HS256"}))

		if err != nil || !token.Valid {
			c.JSON(401, gin.H{"error": "token ไม่ถูกต้อง"})
			c.Abort()
			return
		}

		// ดึง agent_id ใส่ไว้ใน context
		claims := token.Claims.(jwt.MapClaims)
		c.Set("agent_id", claims["agent_id"])
		c.Set("permission_id", claims["permission_id"])

		c.Next() // ไปต่อ
	}
}
