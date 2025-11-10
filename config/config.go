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
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	// AutoMigrate các models
	err = DB.AutoMigrate(
		&models.User{},
		&models.Podcast{},
		&models.ListeningHistory{},
		&models.Flashcard{},
		&models.Note{},
		&models.Document{},
		&models.Favorite{},
		&models.QuizSet{},
		&models.QuizQuestion{},
		&models.QuizOption{},
		&models.QuizAttempt{},
		&models.QuizAttemptHistory{},
		&models.Category{},
		&models.Subject{},
		&models.Tag{},
		&models.Chapter{},
		&models.Notification{},
		&models.Comment{},
		&models.ListeningAnalytics{},
		&models.PodcastAnalytics{},
		&models.SubjectAnalytics{},
		&models.PasswordReset{},
		&models.ListeningEvent{},
	)
	if err != nil {
		log.Fatal("autoMigrate lỗi: ", err)
	}
	log.Println("postgreSQL connected & migrated successfully!")
}

// ConnectDatabase trả về DB instance (dùng cho migration tool)
func ConnectDatabase() (*gorm.DB, error) {
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=Asia/Ho_Chi_Minh",
		dbHost, dbUser, dbPass, dbName, dbPort,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, err
	}

	return db, nil
}
