package ws

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/vnkhanh/e-podcast-backend/utils"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // chỉ để phát triển, nên giới hạn ở production
	},
}

// gửi message dạng JSON qua WebSocket
func sendJSON(conn *websocket.Conn, data interface{}) {
	msg, err := json.Marshal(data)
	if err != nil {
		log.Println("Lỗi JSON marshal:", err)
		return
	}
	if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
		log.Println("Lỗi gửi message:", err)
	}
}

// WebSocket cho tài liệu
func HandleDocumentWebSocket(c *gin.Context) {
	docID := c.Param("id")
	token := c.Query("token")

	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Thiếu token"})
		return
	}
	claims, err := utils.VerifyToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token không hợp lệ hoặc hết hạn"})
		return
	}

	userID := claims.UserID
	log.Printf("Document WS connected: docID=%s, userID=%s\n", docID, userID)

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("WebSocket upgrade thất bại:", err)
		return
	}
	H.Register(docID, conn)
	defer H.Unregister(docID, conn)

	sendJSON(conn, gin.H{"type": "connected", "message": "Connected to document " + docID})

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	log.Printf("Document WS disconnected: docID=%s, userID=%s\n", docID, userID)
	conn.Close()
}

// WebSocket cho global
func HandleGlobalWebSocket(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Thiếu token"})
		return
	}
	claims, err := utils.VerifyToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token không hợp lệ hoặc hết hạn"})
		return
	}

	userID := claims.UserID
	log.Printf("Global WS connected: userID=%s\n", userID)

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("WebSocket upgrade thất bại:", err)
		return
	}
	H.RegisterGlobal(conn)
	defer H.UnregisterGlobal(conn)

	sendJSON(conn, gin.H{"type": "connected", "message": "Connected to global WebSocket"})

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	log.Printf("Global WS disconnected: userID=%s\n", userID)
	conn.Close()
}

// WebSocket riêng cho user
func HandleUserWebSocket(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Thiếu token"})
		return
	}
	claims, err := utils.VerifyToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token không hợp lệ hoặc hết hạn"})
		return
	}

	userID := claims.UserID
	log.Printf("User WS connected: userID=%s\n", userID)

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("WebSocket upgrade thất bại:", err)
		return
	}
	H.RegisterUser(userID, conn)
	defer H.UnregisterUser(userID, conn)

	sendJSON(conn, gin.H{"type": "connected", "message": "Connected to user WebSocket"})

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	log.Printf("User WS disconnected: userID=%s\n", userID)
	conn.Close()
}

// WebSocket cho podcast (bình luận realtime)
func HandlePodcastWebSocket(c *gin.Context) {
	podcastID := c.Param("id")
	token := c.Query("token")

	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Thiếu token"})
		return
	}
	claims, err := utils.VerifyToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token không hợp lệ hoặc hết hạn"})
		return
	}

	userID := claims.UserID
	log.Printf("Podcast WS connected: podcastID=%s, userID=%s\n", podcastID, userID)

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("WebSocket upgrade thất bại:", err)
		return
	}
	H.Register(podcastID, conn)
	defer H.Unregister(podcastID, conn)

	sendJSON(conn, gin.H{"type": "connected", "message": "Connected to podcast " + podcastID})

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	log.Printf("Podcast WS disconnected: podcastID=%s, userID=%s\n", podcastID, userID)
	conn.Close()
}
