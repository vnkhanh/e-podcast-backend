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
	Mutex         sync.RWMutex
}

var H = Hub{
	Clients:       make(map[string]map[*websocket.Conn]*Client),
	GlobalClients: make(map[*websocket.Conn]*Client),
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
func SendStatusUpdate(docID, status string, progress float64, errorMsg string) {
	update := DocumentStatusUpdate{
		DocumentID: docID,
		Status:     status,
		Progress:   progress,
		Error:      errorMsg,
	}
	data, err := json.Marshal(update)
	if err != nil {
		log.Println("JSON marshal error:", err)
		return
	}
	H.Broadcast(docID, websocket.TextMessage, data)
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
	client := h.Clients[docID][conn]
	defer func() {
		conn.WriteMessage(websocket.CloseMessage, []byte{})
		conn.Close()
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
	client := h.GlobalClients[conn]
	defer func() {
		conn.WriteMessage(websocket.CloseMessage, []byte{})
		conn.Close()
	}()
	for msg := range client.Send {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			break
		}
	}
}
