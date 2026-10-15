package handlers

import (
	"checklist-backend/database"
	"checklist-backend/middleware"
	"checklist-backend/models"
	"crypto/subtle"
	"errors"
	"log"

	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

func Login(c *gin.Context) {
	var body struct {
		Login    string `json:"login" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "กรุณากรอก Username และ Password"})
		return
	}

	// หา user จาก DB
	var user models.IamAgents
	err := database.DB.Where("login = ?", body.Login).First(&user).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Println("login:", err)
		c.JSON(500, gin.H{"error": "เกิดข้อผิดพลาดภายในระบบ"})
		return
	}

	// ใช้ข้อความเดียวกันทั้ง user ไม่มี / password ผิด เพื่อไม่ให้เดาได้ว่ามี username นี้หรือไม่
	if err != nil || subtle.ConstantTimeCompare([]byte(user.Password), []byte(body.Password)) != 1 {
		c.JSON(401, gin.H{"error": "Username หรือ Password ไม่ถูกต้อง"})
		return
	}

	// สร้าง JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"agent_id":      user.Agent_id,
		"login":         user.Login,
		"permission_id": user.Permission_id,
		"exp":           time.Now().Add(24 * time.Hour).Unix(),
	})

	tokenString, err := token.SignedString(middleware.JwtSecret)
	if err != nil {
		log.Println("sign token:", err)
		c.JSON(500, gin.H{"error": "เกิดข้อผิดพลาดภายในระบบ"})
		return
	}

	c.JSON(200, gin.H{
		"token": tokenString,
		"agent": gin.H{
			"permission_id": user.Permission_id,
			"title_name_th": user.Title_name_th,
			"first_name_th": user.First_name_th,
			"last_name_th":  user.Last_name_th,
			"login":         user.Login,
		},
	})
}
