// ws/handler.go
package ws

import (
	"log"
	"net/http"

	"github.com/vnkhanh/e-podcast-backend/utils"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Nên kiểm tra Origin ở môi trường production
		return true
	},
}

// WebSocket cho theo dõi tiến trình xử lý tài liệu riêng
func HandleDocumentWebSocket(c *gin.Context) {
	docID := c.Param("id")
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Thiếu token"})
		return
	}

	claims, err := utils.VerifyToken(token)
	if err != nil {
		log.Println("Token không hợp lệ:", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token không hợp lệ hoặc hết hạn"})
		return
	}

	userID := claims.UserID
	log.Printf("WebSocket Document Connect - docID=%s, userID=%s\n", docID, userID)

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("WebSocket upgrade thất bại:", err)
		return
	}

	H.Register(docID, conn)

	if err := conn.WriteMessage(websocket.TextMessage, []byte("Connected to document "+docID)); err != nil {
		H.Unregister(docID, conn)
		conn.Close()
		return
	}

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			log.Printf("WebSocket Document Disconnect - docID=%s, userID=%s\n", docID, userID)
			break
		}
	}

	H.Unregister(docID, conn)
	conn.Close()
}

// WebSocket cho theo dõi trạng thái danh sách tài liệu chung
func HandleGlobalWebSocket(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Thiếu token"})
		return
	}

	claims, err := utils.VerifyToken(token)
	if err != nil {
		log.Println("Token không hợp lệ:", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token không hợp lệ hoặc hết hạn"})
		return
	}

	userID := claims.UserID
	log.Printf("WebSocket Global Connect - userID=%s\n", userID)

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("WebSocket upgrade thất bại:", err)
		return
	}

	H.RegisterGlobal(conn)

	if err := conn.WriteMessage(websocket.TextMessage, []byte("Connected to document list")); err != nil {
		H.UnregisterGlobal(conn)
		conn.Close()
		return
	}

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			log.Printf("WebSocket Global Disconnect - userID=%s\n", userID)
			break
		}
	}

	H.UnregisterGlobal(conn)
	conn.Close()
}
