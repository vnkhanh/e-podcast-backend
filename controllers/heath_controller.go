package controllers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vnkhanh/e-podcast-backend/config"
	"github.com/vnkhanh/e-podcast-backend/ws"
)

func HealthCheck(c *gin.Context) {
	db := config.DB

	// Mặc định trạng thái OK
	response := gin.H{
		"status":    "ok",
		"message":   "Service is healthy",
		"timestamp": time.Now().Unix(),
		"db":        "ok",
		"websocket": gin.H{
			"enabled": true,
			"stats":   ws.H.GetStats(),
		},
	}

	// Thử ping database
	sqlDB, err := db.DB()
	if err != nil {
		response["db"] = "error: cannot get DB instance"
		response["status"] = "degraded"
		c.JSON(http.StatusInternalServerError, response)
		return
	}

	if err := sqlDB.Ping(); err != nil {
		response["db"] = "error: cannot connect to DB"
		response["status"] = "degraded"
		c.JSON(http.StatusInternalServerError, response)
		return
	}

	// Trả về nếu mọi thứ ổn
	c.JSON(http.StatusOK, response)
}
