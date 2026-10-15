package database

import (
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Connect() {
	dsn := "host=172.21.140.92 user=app_dcc  password=DeeCeeC@92 dbname=oraiec1 port=5432 sslmode=disable TimeZone=Asia/Bangkok"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})

	if err != nil {
		log.Fatal("เชื่อมต่อ DB ไม่ได้:", err)
	}

	DB = db
	log.Println("เชื่อมต่อ DB สำเร็จ!")
}
