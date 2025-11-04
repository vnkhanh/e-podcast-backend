package ws

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

type Client struct {
	Conn *websocket.Conn
	Send chan []byte
}

type Hub struct {
	Clients       map[string]map[*websocket.Conn]*Client // Theo từng documentID
	GlobalClients map[*websocket.Conn]*Client            // Dành cho broadcast chung
	UserClients   map[string]map[*websocket.Conn]*Client // Theo từng userID
	Mutex         sync.RWMutex
}

var H = Hub{
	Clients:       make(map[string]map[*websocket.Conn]*Client),
	GlobalClients: make(map[*websocket.Conn]*Client),
	UserClients:   make(map[string]map[*websocket.Conn]*Client),
}

// Struct gửi trạng thái tiến trình của 1 tài liệu
type DocumentStatusUpdate struct {
	DocumentID string  `json:"document_id"`
	Status     string  `json:"status"`
	Progress   float64 `json:"progress"`
	Error      string  `json:"error,omitempty"`
}

// Register theo documentID riêng
func (h *Hub) Register(docID string, conn *websocket.Conn) {
	h.Mutex.Lock()
	defer h.Mutex.Unlock()

	if _, ok := h.Clients[docID]; !ok {
		h.Clients[docID] = make(map[*websocket.Conn]*Client)
	}

	client := &Client{
		Conn: conn,
		Send: make(chan []byte, 256),
	}

	h.Clients[docID][conn] = client

	go h.readPump(docID, conn)
	go h.writePump(docID, conn)
}

// Register global cho trang danh sách
func (h *Hub) RegisterGlobal(conn *websocket.Conn) {
	h.Mutex.Lock()
	defer h.Mutex.Unlock()

	client := &Client{
		Conn: conn,
		Send: make(chan []byte, 256),
	}

	h.GlobalClients[conn] = client

	go h.readGlobalPump(conn)
	go h.writeGlobalPump(conn)
}

// Đăng ký kết nối theo userID
func (h *Hub) RegisterUser(userID string, conn *websocket.Conn) {
	h.Mutex.Lock()
	defer h.Mutex.Unlock()

	// Nếu user đã có connection cũ thì đóng hết
	if clients, ok := h.UserClients[userID]; ok {
		for oldConn, client := range clients {
			close(client.Send)
			oldConn.Close()
			delete(clients, oldConn)
		}
	}

	if _, ok := h.UserClients[userID]; !ok {
		h.UserClients[userID] = make(map[*websocket.Conn]*Client)
	}

	client := &Client{
		Conn: conn,
		Send: make(chan []byte, 256),
	}

	h.UserClients[userID][conn] = client

	go h.readUserPump(userID, conn)
	go h.writeUserPump(userID, conn)

	log.Printf("RegisterUser: %s (%d connections active)", userID, len(h.UserClients[userID]))
}

// Gửi message đến tất cả kết nối của 1 user
func (h *Hub) BroadcastToUser(userID string, messageType int, data []byte) {
	h.Mutex.RLock()
	defer h.Mutex.RUnlock()

	if clients, ok := h.UserClients[userID]; ok {
		for _, client := range clients {
			select {
			case client.Send <- data:
			default:
			}
		}
	}
}

// Huỷ đăng ký theo userID
func (h *Hub) UnregisterUser(userID string, conn *websocket.Conn) {
	h.Mutex.Lock()
	defer h.Mutex.Unlock()

	if clients, ok := h.UserClients[userID]; ok {
		if client, ok := clients[conn]; ok {
			close(client.Send)
			delete(clients, conn)
		}
		if len(clients) == 0 {
			delete(h.UserClients, userID)
		}
	}
}

func (h *Hub) readUserPump(userID string, conn *websocket.Conn) {
	defer h.UnregisterUser(userID, conn)
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (h *Hub) writeUserPump(userID string, conn *websocket.Conn) {
	h.Mutex.RLock()
	clientsMap, ok := h.UserClients[userID]
	if !ok {
		h.Mutex.RUnlock()
		return
	}
	client, ok := clientsMap[conn]
	h.Mutex.RUnlock()
	if !ok {
		return
	}

	defer func() {
		conn.WriteMessage(websocket.CloseMessage, []byte{})
		conn.Close()
		h.UnregisterUser(userID, conn)
	}()

	for msg := range client.Send {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			break
		}
	}
}

func SendBadgeUpdate(userID string, unreadCount int64) {
	update := map[string]interface{}{
		"type":         "badge_update",
		"unread_count": unreadCount,
	}
	data, _ := json.Marshal(update)
	H.BroadcastToUser(userID, websocket.TextMessage, data)
}

// Broadcast theo documentID
func (h *Hub) Broadcast(docID string, messageType int, data []byte) {
	h.Mutex.RLock()
	defer h.Mutex.RUnlock()

	if clients, ok := h.Clients[docID]; ok {
		for _, client := range clients {
			select {
			case client.Send <- data:
			default:
			}
		}
	}
}

// Broadcast toàn bộ global clients (danh sách)
func (h *Hub) BroadcastGlobal(messageType int, data []byte) {
	h.Mutex.RLock()
	defer h.Mutex.RUnlock()

	for _, client := range h.GlobalClients {
		select {
		case client.Send <- data:
		default:
		}
	}
}

// Public function gọi gửi status tài liệu
// Gửi trạng thái document (status + progress + error)
// docID: ID tài liệu
// status: trạng thái hiện tại
// progress: % tiến trình (0-100)
// errorMsg: lỗi nếu có
func SendStatusUpdate(docID, status string, progress float64, errorMsg string) {
	update := map[string]interface{}{
		"type":        "document_status_update",
		"document_id": docID,
		"status":      status,
		"progress":    progress,
	}
	if errorMsg != "" {
		update["error"] = errorMsg
	}

	data, err := json.Marshal(update)
	if err != nil {
		log.Println("JSON marshal error:", err)
		return
	}

	// Gửi tới client đang xem document
	H.Broadcast(docID, websocket.TextMessage, data)
	// Gửi luôn tới global client (danh sách document)
	H.BroadcastGlobal(websocket.TextMessage, data)
}

// Public function gửi signal cập nhật danh sách tài liệu
func BroadcastDocumentListChanged() {
	data := []byte(`{"type": "document_list_changed"}`)
	H.BroadcastGlobal(websocket.TextMessage, data)
}

// Unregister client theo documentID
func (h *Hub) Unregister(docID string, conn *websocket.Conn) {
	h.Mutex.Lock()
	defer h.Mutex.Unlock()

	if clients, ok := h.Clients[docID]; ok {
		if client, ok := clients[conn]; ok {
			close(client.Send)
			delete(clients, conn)
		}
		if len(clients) == 0 {
			delete(h.Clients, docID)
		}
	}
}

// Unregister global client
func (h *Hub) UnregisterGlobal(conn *websocket.Conn) {
	h.Mutex.Lock()
	defer h.Mutex.Unlock()

	if client, ok := h.GlobalClients[conn]; ok {
		close(client.Send)
		delete(h.GlobalClients, conn)
	}
}

// Read pump riêng theo documentID
func (h *Hub) readPump(docID string, conn *websocket.Conn) {
	defer h.Unregister(docID, conn)
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// Write pump riêng theo documentID
func (h *Hub) writePump(docID string, conn *websocket.Conn) {
	h.Mutex.RLock()
	clientsMap, ok := h.Clients[docID]
	if !ok {
		h.Mutex.RUnlock()
		return
	}
	client, ok := clientsMap[conn]
	h.Mutex.RUnlock()
	if !ok {
		return
	}

	defer func() {
		conn.WriteMessage(websocket.CloseMessage, []byte{})
		conn.Close()
		h.Unregister(docID, conn)
	}()

	for msg := range client.Send {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			break
		}
	}
}

// Read pump global
func (h *Hub) readGlobalPump(conn *websocket.Conn) {
	defer h.UnregisterGlobal(conn)
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// Write pump global
func (h *Hub) writeGlobalPump(conn *websocket.Conn) {
	h.Mutex.RLock()
	client, ok := h.GlobalClients[conn]
	h.Mutex.RUnlock()
	if !ok {
		// client chưa được đăng ký hoặc đã bị xóa
		return
	}

	defer func() {
		conn.WriteMessage(websocket.CloseMessage, []byte{})
		conn.Close()
		h.UnregisterGlobal(conn)
	}()

	for msg := range client.Send {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			break
		}
	}
}
