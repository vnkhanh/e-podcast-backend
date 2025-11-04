package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/vnkhanh/e-podcast-backend/config"
	"github.com/vnkhanh/e-podcast-backend/models"
	"github.com/vnkhanh/e-podcast-backend/ws"
	"gorm.io/gorm"
)

// Gửi thông báo realtime + lưu DB
func notifyComment(db *gorm.DB, userID uuid.UUID, title, message, notifType string) {
	notif := models.Notification{
		UserID:  userID,
		Title:   title,
		Message: message,
		Type:    notifType,
	}
	db.Create(&notif)

	// Gửi realtime notification
	data := map[string]interface{}{
		"type":    notifType,
		"title":   title,
		"message": message,
	}
	jsonData, _ := json.Marshal(data)
	ws.H.BroadcastToUser(userID.String(), websocket.TextMessage, jsonData)

	// Cập nhật badge số lượng chưa đọc
	var count int64
	db.Model(&models.Notification{}).Where("user_id = ? AND is_read = false", userID).Count(&count)
	ws.SendBadgeUpdate(userID.String(), count)
}

// Request tạo bình luận
type CreateCommentRequest struct {
	PodcastID string  `json:"podcast_id" binding:"required"`
	Content   string  `json:"content" binding:"required"`
	ParentID  *string `json:"parent_id,omitempty"`
}

// Tạo bình luận hoặc trả lời
func CreateComment(c *gin.Context) {
	var req CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Lấy user từ context
	userIDStr, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Không tìm thấy thông tin người dùng"})
		return
	}
	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
		return
	}

	// Lấy thông tin người dùng (bao gồm role)
	var user models.User
	if err := config.DB.Select("id", "full_name", "role").First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy thông tin người dùng"})
		return
	}

	// Parse podcast_id
	podcastID, err := uuid.Parse(req.PodcastID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "PodcastID không hợp lệ"})
		return
	}

	// Parse parent_id nếu có
	var parentID *uuid.UUID
	if req.ParentID != nil {
		if id, err := uuid.Parse(*req.ParentID); err == nil {
			parentID = &id
		}
	}

	// Tạo comment
	comment := models.Comment{
		PodcastID: podcastID,
		UserID:    userID,
		Content:   req.Content,
		ParentID:  parentID,
	}
	if err := config.DB.Create(&comment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lưu bình luận"})
		return
	}

	// Load user vào comment (Preload sẽ lấy đầy đủ trường user)
	config.DB.Preload("User").First(&comment, "id = ?", comment.ID)

	// Format dữ liệu phản hồi (bao gồm parent_id và replies rỗng)
	role := ""
	switch comment.User.Role {
	case "admin":
		role = "Quản trị viên"
	case "teacher":
		role = "Giảng viên"
	}

	var parentVal interface{} = nil
	if comment.ParentID != nil {
		parentVal = comment.ParentID.String()
	}

	response := map[string]interface{}{
		"id":         comment.ID.String(),
		"podcast_id": comment.PodcastID.String(),
		"user_id":    comment.UserID.String(),
		"user_name":  comment.User.FullName,
		"user_role":  role,
		"content":    comment.Content,
		"created_at": comment.CreatedAt.Format("02/01/2006 15:04"),
		"parent_id":  parentVal,
		"replies":    []interface{}{},
	}

	// Gửi realtime tới tất cả client đang xem podcast
	data := map[string]interface{}{
		"type":       "new_comment",
		"podcast_id": podcastID.String(),
		"comment":    response,
	}
	wsData, _ := json.Marshal(data)
	ws.H.Broadcast(podcastID.String(), websocket.TextMessage, wsData)

	// Thông báo cho chủ podcast
	var podcast models.Podcast
	if err := config.DB.First(&podcast, "id = ?", podcastID).Error; err == nil {
		if podcast.CreatedBy != userID {
			title := "Bình luận mới về podcast của bạn"
			message := user.FullName + " đã bình luận: " + req.Content
			notifyComment(config.DB, podcast.CreatedBy, title, message, "comment_notification")
		}
	}

	// Nếu là reply, thông báo cho người bị reply
	if parentID != nil {
		var parent models.Comment
		if err := config.DB.Preload("User").First(&parent, "id = ?", *parentID).Error; err == nil {
			if parent.UserID != userID {
				title := "Ai đó đã trả lời bình luận của bạn"
				message := user.FullName + " đã trả lời: " + req.Content
				notifyComment(config.DB, parent.UserID, title, message, "reply_notification")
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Bình luận thành công",
		"data":    response,
	})
}

// Lấy tất cả bình luận (đệ quy nhiều cấp)
func GetComments(c *gin.Context) {
	podcastID := c.Param("id")
	var rootComments []models.Comment

	if err := config.DB.
		Preload("User").
		Where("podcast_id = ? AND parent_id IS NULL", podcastID).
		Order("created_at ASC").
		Find(&rootComments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy bình luận"})
		return
	}

	// Đệ quy lấy replies
	var loadReplies func(comment *models.Comment)
	loadReplies = func(comment *models.Comment) {
		var replies []models.Comment
		config.DB.
			Preload("User").
			Where("parent_id = ?", comment.ID).
			Order("created_at ASC").
			Find(&replies)

		for i := range replies {
			loadReplies(&replies[i])
		}

		comment.Replies = replies
	}

	for i := range rootComments {
		loadReplies(&rootComments[i])
	}

	// Format trả về
	type CommentResponse struct {
		ID        uuid.UUID         `json:"id"`
		PodcastID uuid.UUID         `json:"podcast_id"`
		UserID    uuid.UUID         `json:"user_id"`
		UserName  string            `json:"user_name"`
		UserRole  string            `json:"user_role"`
		Content   string            `json:"content"`
		CreatedAt string            `json:"created_at"`
		Replies   []CommentResponse `json:"replies,omitempty"`
	}

	var format func(models.Comment) CommentResponse
	format = func(cmt models.Comment) CommentResponse {
		role := ""
		switch cmt.User.Role {
		case "admin":
			role = "Quản trị viên"
		case "teacher":
			role = "Giảng viên"
		}

		resp := CommentResponse{
			ID:        cmt.ID,
			PodcastID: cmt.PodcastID,
			UserID:    cmt.UserID,
			UserName:  cmt.User.FullName,
			UserRole:  role,
			Content:   cmt.Content,
			CreatedAt: cmt.CreatedAt.Format("02/01/2006 15:04"),
		}

		for _, reply := range cmt.Replies {
			resp.Replies = append(resp.Replies, format(reply))
		}

		return resp
	}

	var response []CommentResponse
	for _, root := range rootComments {
		response = append(response, format(root))
	}

	c.JSON(http.StatusOK, response)
}

// Xóa bình luận hoặc trả lời (xóa luôn toàn bộ reply con)
func DeleteComment(c *gin.Context) {
	db := config.DB
	commentID := c.Param("id")

	userIDStr, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Không tìm thấy người dùng"})
		return
	}
	userID, _ := uuid.Parse(userIDStr.(string))

	var comment models.Comment
	if err := db.First(&comment, "id = ?", commentID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy bình luận"})
		return
	}

	// Kiểm tra quyền xóa
	if comment.UserID != userID {
		var user models.User
		if err := db.Select("role").First(&user, "id = ?", userID).Error; err != nil || user.Role != models.RoleAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "Bạn không có quyền xóa bình luận này"})
			return
		}
	}

	// Hàm đệ quy xóa toàn bộ reply con
	var deleteReplies func(parentID uuid.UUID)
	deleteReplies = func(parentID uuid.UUID) {
		var replies []models.Comment
		if err := db.Where("parent_id = ?", parentID).Find(&replies).Error; err == nil {
			for _, r := range replies {
				deleteReplies(r.ID)
				db.Delete(&r)
			}
		}
	}

	// Xóa các reply trước
	deleteReplies(comment.ID)

	// Xóa comment chính
	if err := db.Delete(&comment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xóa bình luận"})
		return
	}

	// GỬI REALTIME EVENT "delete_comment" với comment_id dạng STRING
	data := map[string]interface{}{
		"type":       "delete_comment",
		"comment_id": comment.ID.String(), // QUAN TRỌNG: phải convert UUID sang string
		"podcast_id": comment.PodcastID.String(),
	}
	jsonData, _ := json.Marshal(data)
	ws.H.Broadcast(comment.PodcastID.String(), websocket.TextMessage, jsonData)

	c.JSON(http.StatusOK, gin.H{"message": "Đã xóa bình luận và toàn bộ trả lời con"})
}
