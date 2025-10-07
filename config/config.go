package config

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/vnkhanh/e-podcast-backend/models"
)

var DB *gorm.DB

func InitDB() {
	// Lấy thông tin từ biến môi trường
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	// DSN cho PostgreSQL
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=Asia/Ho_Chi_Minh",
		dbHost, dbUser, dbPass, dbName, dbPort,
	)

	// Kết nối DB với logger
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatal("Không thể kết nối database:", err)
	}

	DB = db

	// Lấy *sql.DB để config connection pooling
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal("Không thể lấy sql.DB từ gorm:", err)
	}

	// Connection Pooling config
	sqlDB.SetMaxIdleConns(10)                  // số kết nối rảnh tối đa
	sqlDB.SetMaxOpenConns(100)                 // số kết nối mở tối đa
	sqlDB.SetConnMaxLifetime(time.Hour)        // tuổi thọ tối đa của 1 kết nối
	sqlDB.SetConnMaxIdleTime(10 * time.Minute) // idle timeout

	// AutoMigrate các models
	err = DB.AutoMigrate(
		&models.User{},
		&models.Podcast{},
		&models.PodcastShare{},
		&models.Flashcard{},
		&models.Note{},
		&models.Document{},
		&models.Favorite{},
		&models.QuizQuestion{},
		&models.QuizOption{},
		&models.QuizAttempt{},
		&models.Topic{},
		&models.Category{},
		&models.Subject{},
		&models.Tag{},
		&models.Chapter{},
	)
	if err != nil {
		log.Fatal("autoMigrate lỗi: ", err)
	}
	log.Println("postgreSQL connected & migrated successfully!")
}
