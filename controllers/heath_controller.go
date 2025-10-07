package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vnkhanh/e-podcast-backend/config"
)

func HealthCheck(c *gin.Context) {
	db := config.DB

	// Mặc định trạng thái OK
	response := gin.H{
		"status":  "ok",
		"message": "Service is healthy",
		"db":      "ok",
	}

	// Thử ping database
	sqlDB, err := db.DB()
	if err != nil {
		response["db"] = "error: cannot get DB instance"
		c.JSON(http.StatusInternalServerError, response)
		return
	}

	if err := sqlDB.Ping(); err != nil {
		response["db"] = "error: cannot connect to DB"
		c.JSON(http.StatusInternalServerError, response)
		return
	}

	// Trả về nếu mọi thứ ổn
	c.JSON(http.StatusOK, response)
}
