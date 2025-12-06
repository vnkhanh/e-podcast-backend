package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/vnkhanh/e-podcast-backend/utils"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Giới hạn origins trong production
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Gửi message dạng JSON qua WebSocket
func sendJSON(conn *websocket.Conn, data interface{}) error {
	msg, err := json.Marshal(data)
	if err != nil {
		log.Println("Lỗi JSON marshal:", err)
		return err
	}
	
	conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
		log.Println("Lỗi gửi message:", err)
		return err
	}
	return nil
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
	defer func() {
		H.Unregister(docID, conn)
		conn.Close()
		log.Printf("Document WS disconnected: docID=%s, userID=%s\n", docID, userID)
	}()

	// Configure connection
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	sendJSON(conn, gin.H{"type": "connected", "message": "Connected to document " + docID})

	// Ping ticker
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	// Channel để signal khi có lỗi đọc
	done := make(chan struct{})

	// Goroutine đọc messages
	go func() {
		defer close(done)
		for {
			conn.SetReadLimit(maxMessageSize)
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("Document WS error: %v", err)
				}
				break
			}
			// Xử lý ping từ client
			var msg map[string]interface{}
			if json.Unmarshal(message, &msg) == nil {
				if msgType, ok := msg["type"].(string); ok && msgType == "ping" {
					sendJSON(conn, gin.H{"type": "pong"})
				}
			}
		}
	}()

	// Goroutine gửi ping
	for {
		select {
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-done:
			return
		}
	}
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
	defer func() {
		H.UnregisterGlobal(conn)
		conn.Close()
		log.Printf("Global WS disconnected: userID=%s\n", userID)
	}()

	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	sendJSON(conn, gin.H{"type": "connected", "message": "Connected to global WebSocket"})

	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			conn.SetReadLimit(maxMessageSize)
			_, message, err := conn.ReadMessage()
			if err != nil {
				break
			}
			var msg map[string]interface{}
			if json.Unmarshal(message, &msg) == nil {
				if msgType, ok := msg["type"].(string); ok && msgType == "ping" {
					sendJSON(conn, gin.H{"type": "pong"})
				}
			}
		}
	}()

	for {
		select {
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-done:
			return
		}
	}
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
	defer func() {
		H.UnregisterUser(userID, conn)
		conn.Close()
		log.Printf("User WS disconnected: userID=%s\n", userID)
	}()

	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	sendJSON(conn, gin.H{"type": "connected", "message": "Connected to user WebSocket"})

	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			conn.SetReadLimit(maxMessageSize)
			_, message, err := conn.ReadMessage()
			if err != nil {
				break
			}
			var msg map[string]interface{}
			if json.Unmarshal(message, &msg) == nil {
				if msgType, ok := msg["type"].(string); ok && msgType == "ping" {
					sendJSON(conn, gin.H{"type": "pong"})
				}
			}
		}
	}()

	for {
		select {
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

// WebSocket cho podcast (bình luận realtime) - QUAN TRỌNG NHẤT
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
	defer func() {
		H.Unregister(podcastID, conn)
		conn.Close()
		log.Printf("Podcast WS disconnected: podcastID=%s, userID=%s\n", podcastID, userID)
	}()

	// Configure connection với timeouts
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Gửi connected message
	sendJSON(conn, gin.H{"type": "connected", "message": "Connected to podcast " + podcastID})

	// Ticker để gửi ping định kỳ
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	// Channel để signal khi có lỗi đọc
	done := make(chan struct{})

	// Goroutine đọc messages từ client
	go func() {
		defer close(done)
		for {
			conn.SetReadLimit(maxMessageSize)
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("Podcast WS read error: %v", err)
				}
				break
			}
			
			// Xử lý ping message từ client
			var msg map[string]interface{}
			if json.Unmarshal(message, &msg) == nil {
				if msgType, ok := msg["type"].(string); ok && msgType == "ping" {
					sendJSON(conn, gin.H{"type": "pong"})
				}
			}
		}
	}()

	// Main loop: gửi ping định kỳ
	for {
		select {
		case <-ticker.C:
			// Gửi ping message
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-done:
			// Client đã disconnect
			return
		}
	}
}
