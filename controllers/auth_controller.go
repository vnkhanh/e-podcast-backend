package controllers

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"cloud.google.com/go/auth/credentials/idtoken"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/vnkhanh/e-podcast-backend/config"
	"github.com/vnkhanh/e-podcast-backend/models"
	"github.com/vnkhanh/e-podcast-backend/utils"
)

// ====== INPUT STRUCTS ======
type RegisterInput struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	FullName string `json:"full_name" binding:"required"`
}

type LoginInput struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// Helper tạo con trỏ bool
func BoolPtr(b bool) *bool { return &b }

// ====== HANDLERS ======
func Register(c *gin.Context) {
	var input RegisterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check email tồn tại
	var existing models.User
	if err := config.DB.Where("email = ?", input.Email).First(&existing).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email đã được sử dụng"})
		return
	}

	// Hash password
	hashed, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể mã hoá mật khẩu"})
		return
	}

	// Tạo user mới
	newUser := models.User{
		// ID sẽ tự sinh vì default:gen_random_uuid()
		FullName: input.FullName,
		Email:    input.Email,
		Password: string(hashed),
		Role:     models.RoleUser,
		Status:   BoolPtr(true), // default true
	}

	if err := config.DB.Create(&newUser).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi khi tạo người dùng"})
		return
	}

	// Ẩn mật khẩu khi trả về
	newUser.Password = ""
	c.JSON(http.StatusCreated,
		gin.H{
			"message": "Đăng ký thành công",
			"user":    newUser,
		})
}

func Login(c *gin.Context) {
	var input LoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user models.User
	if err := config.DB.Where("email = ?", input.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Email hoặc mật khẩu không đúng"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Email hoặc mật khẩu không đúng"})
		return
	}
	if user.Status != nil && !*user.Status {
		c.JSON(http.StatusForbidden, gin.H{"error": "Tài khoản của bạn đã bị tạm khóa"})
		return
	}

	// Sinh JWT (truyền ID dạng string và Role)
	token, err := utils.GenerateToken(user.ID.String(), string(user.Role))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Đăng nhập thành công",
		"token":   token,
		"user": gin.H{
			"id":        user.ID,
			"email":     user.Email,
			"full_name": user.FullName,
			"role":      user.Role,
		},
	})
}

type GoogleLoginInput struct {
	IDToken string `json:"id_token" binding:"required"`
}

func GoogleLogin(c *gin.Context) {
	var input GoogleLoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Xác minh token với đúng GOOGLE_CLIENT_ID
	payload, err := idtoken.Validate(c, input.IDToken, os.Getenv("GOOGLE_CLIENT_ID"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token Google không hợp lệ"})
		return
	}

	// Lấy thông tin từ payload
	email, _ := payload.Claims["email"].(string)
	fullName, _ := payload.Claims["name"].(string)

	// Tìm user trong DB
	var user models.User
	if err := config.DB.Where("email = ?", email).First(&user).Error; err != nil {
		// Nếu chưa có -> tạo mới
		user = models.User{
			// ID tự sinh nhờ default:gen_random_uuid(), nhưng vẫn có thể chỉ định nếu muốn
			ID:       uuid.New(),
			Email:    email,
			FullName: fullName,
			Role:     models.RoleUser,
			Status:   BoolPtr(true), // default true
			// Password để trống vì login Google
		}
		if err := config.DB.Create(&user).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo user Google"})
			return
		}
	}
	if user.Status != nil && !*user.Status {
		c.JSON(http.StatusForbidden, gin.H{"error": "Tài khoản của bạn đã bị tạm khóa"})
		return
	}

	// Tạo JWT token: truyền ID dạng string
	token, err := utils.GenerateToken(user.ID.String(), string(user.Role))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user": gin.H{
			"id":        user.ID,
			"email":     user.Email,
			"full_name": user.FullName,
			"role":      user.Role,
		},
	})
}

// ========== QUÊN MẬT KHẨU ==========
// ForgotPassword tạo JWT token gửi qua email
func ForgotPassword(c *gin.Context) {
	type Request struct {
		Email string `json:"email" binding:"required,email"`
	}
	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := config.DB
	var user models.User
	if err := db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		// Không lộ email tồn tại hay không
		c.JSON(http.StatusOK, gin.H{"message": "Nếu email tồn tại, link đổi mật khẩu đã được gửi"})
		return
	}

	// ===== Tạo JWT reset token =====
	secret := []byte(os.Getenv("JWT_SECRET"))
	expiresAt := time.Now().Add(15 * time.Minute) // token hết hạn sau 15 phút
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": user.ID.String(),
		"exp":     expiresAt.Unix(),
	})
	resetToken, _ := token.SignedString(secret)

	// ===== Tạo link và body email =====
	link := fmt.Sprintf(`%s/auth/reset-password?token=%s`, os.Getenv("FE_BASE_URL"), resetToken)
	body := fmt.Sprintf(`
	<p>Click vào link dưới đây để đổi mật khẩu:</p>
	<p><a href="%s">Đổi mật khẩu</a></p>
	<p>Token này sẽ hết hạn vào <b>%s</b></p>
	<p>Nếu bạn không yêu cầu đổi mật khẩu, hãy bỏ qua email này.</p>
	`, link, expiresAt.Format("02/01/2006 15:04")) // định dạng: dd/mm/yyyy HH:mm

	// ===== Gửi email =====
	if err := utils.SendEmail(user.Email, "Quên mật khẩu", body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể gửi email"})
		return
	}

	// ===== Trả response =====
	c.JSON(http.StatusOK, gin.H{"message": "Nếu email tồn tại, link đổi mật khẩu đã được gửi"})
}

// ResetPassword dùng token JWT để cập nhật mật khẩu
func ResetPassword(c *gin.Context) {
	type Request struct {
		Token       string `json:"token" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}
	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	secret := []byte(os.Getenv("JWT_SECRET"))
	parsedToken, err := jwt.Parse(req.Token, func(token *jwt.Token) (interface{}, error) { return secret, nil })
	if err != nil || !parsedToken.Valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token không hợp lệ hoặc hết hạn"})
		return
	}

	claims := parsedToken.Claims.(jwt.MapClaims)
	userID := claims["user_id"].(string)

	hashed, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	db := config.DB
	db.Model(&models.User{}).Where("id = ?", userID).Update("password", string(hashed))

	c.JSON(http.StatusOK, gin.H{"message": "Đổi mật khẩu thành công"})
}

// ==== ADMIN TẠO GIẢNG VIÊN ====
type CreateLecturerInput struct {
	FullName string `json:"full_name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

func AdminCreateLecturer(c *gin.Context) {
	role := c.GetString("role")
	if role != string(models.RoleAdmin) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Chỉ admin mới có quyền tạo giảng viên"})
		return
	}

	var input CreateLecturerInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Kiểm tra email trùng
	var existing models.User
	if err := config.DB.Where("email = ?", input.Email).First(&existing).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email đã tồn tại"})
		return
	}

	// Mã hoá mật khẩu
	hashed, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể mã hoá mật khẩu"})
		return
	}

	// Tạo tài khoản giảng viên
	newUser := models.User{
		FullName: input.FullName,
		Email:    input.Email,
		Password: string(hashed),
		Role:     models.RoleLecturer,
		Status:   BoolPtr(true), // default true
	}

	if err := config.DB.Create(&newUser).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo tài khoản"})
		return
	}

	// Gửi email thông báo (không chặn luồng)
	go func() {
		subject := "Tài khoản giảng viên E-Podcast của bạn đã được tạo"
		body := `
		<h3>Xin chào ` + input.FullName + `,</h3>
		<p>Bạn đã được cấp tài khoản giảng viên trên hệ thống <b>E-Podcast</b>.</p>
		<p><b>Email đăng nhập:</b> ` + input.Email + `<br>
		<b>Mật khẩu:</b> ` + input.Password + `</p>
		<p>Vui lòng đăng nhập và đổi mật khẩu sau khi sử dụng lần đầu.</p>
		<hr>
		<p><i>Đây là email tự động, vui lòng không trả lời.</i></p>
		`
		if err := utils.SendEmail(input.Email, subject, body); err != nil {
			// In log lỗi, không ảnh hưởng đến API chính
			println("Lỗi gửi email:", err.Error())
		}
	}()

	c.JSON(http.StatusCreated, gin.H{
		"message": "Tạo giảng viên thành công",
		"user": gin.H{
			"id":        newUser.ID,
			"full_name": newUser.FullName,
			"email":     newUser.Email,
			"role":      newUser.Role,
		},
	})
}

// Đổi mật khẩu
type ChangePasswordInput struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

func ChangePassword(c *gin.Context) {
	db := config.DB
	userID := c.GetString("user_id")

	var input ChangePasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Lấy user hiện tại
	var user models.User
	if err := db.Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Người dùng không tồn tại"})
		return
	}

	// Kiểm tra mật khẩu cũ
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.OldPassword)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Mật khẩu cũ không đúng"})
		return
	}

	// Mã hoá mật khẩu mới
	hashed, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể mã hoá mật khẩu mới"})
		return
	}

	// Cập nhật DB
	user.Password = string(hashed)
	if err := db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi khi cập nhật mật khẩu"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Đổi mật khẩu thành công",
	})
}

func AdminGetUsers(c *gin.Context) {
	db := config.DB

	// --- Lấy query params ---
	name := c.Query("name")
	role := c.Query("role")
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "10")

	page, _ := strconv.Atoi(pageStr)
	limit, _ := strconv.Atoi(limitStr)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	var users []models.User
	query := db.Model(&models.User{})

	// --- Chỉ lấy student và teacher ---
	query = query.Where("role IN ?", []models.UserRole{models.RoleUser, models.RoleLecturer})

	// --- Lọc theo tên ---
	if name != "" {
		query = query.Where("full_name LIKE ?", "%"+name+"%")
	}

	// --- Lọc theo role (nếu có) ---
	if role != "" {
		if role == string(models.RoleLecturer) || role == string(models.RoleUser) {
			query = query.Where("role = ?", role)
		}
	}

	// --- Đếm tổng số bản ghi ---
	var total int64
	query.Count(&total)

	// --- Phân trang ---
	offset := (page - 1) * limit
	if err := query.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi khi lấy danh sách người dùng"})
		return
	}

	// --- Ẩn mật khẩu ---
	for i := range users {
		users[i].Password = ""
	}

	// --- Trả JSON ---
	c.JSON(http.StatusOK, gin.H{
		"users": users,
		"pagination": gin.H{
			"page":      page,
			"limit":     limit,
			"total":     total,
			"totalPage": int(math.Ceil(float64(total) / float64(limit))),
		},
	})
}

func AdminGetUserDetail(c *gin.Context) {
	db := config.DB
	userID := c.Param("id")

	// Parse UUID
	id, err := uuid.Parse(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID người dùng không hợp lệ"})
		return
	}

	var user models.User
	if err := db.Preload("Documents").
		Preload("Favorites").
		Preload("Notes").
		Preload("Flashcards").
		First(&user, "id = ?", id).Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy người dùng"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi khi lấy thông tin người dùng"})
		return
	}

	// Ẩn mật khẩu
	user.Password = ""

	c.JSON(http.StatusOK, gin.H{"user": user})
}

func AdminDeleteUser(c *gin.Context) {
	db := config.DB
	userID := c.Param("id")

	// Parse UUID
	id, err := uuid.Parse(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID người dùng không hợp lệ"})
		return
	}

	// Tìm user
	var user models.User
	if err := db.First(&user, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy người dùng"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi khi truy vấn người dùng"})
		return
	}

	// Không cho xoá admin
	if user.Role == models.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "Không thể xoá tài khoản admin"})
		return
	}

	// Xoá người dùng (tự động xoá liên kết nếu có OnDelete:CASCADE)
	if err := db.Delete(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi khi xoá người dùng"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Xoá người dùng thành công",
		"user_id": user.ID,
	})
}

// PATCH /admin/users/:id/toggle-status
func ToggleUserStatus(c *gin.Context) {
	idParam := c.Param("id")
	userID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	var user models.User
	if err := config.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy người dùng"})
		return
	}

	// đảo trạng thái
	if user.Status == nil {
		defaultStatus := true
		user.Status = &defaultStatus
	} else {
		v := !*user.Status
		user.Status = &v
	}

	if err := config.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Cập nhật trạng thái thành công",
		"user":    user,
	})
}

// Nếu muốn lấy user từ JWT token hiện tại
func GetProfileUser(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Chưa đăng nhập"})
		return
	}

	var user models.User
	db := c.MustGet("db").(*gorm.DB)
	if err := db.Preload("Documents").
		Preload("Favorites").
		Preload("Notes").
		Preload("Flashcards").
		First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": user})
}

// type FacebookLoginInput struct {
// 	AccessToken string `json:"access_token" binding:"required"`
// }

// func FacebookLogin(c *gin.Context) {
// 	var input FacebookLoginInput
// 	if err := c.ShouldBindJSON(&input); err != nil {
// 		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
// 		return
// 	}

// 	// Gọi Graph API để verify access_token và lấy thông tin user
// 	resp, err := http.Get("https://graph.facebook.com/me?fields=id,name,email&access_token=" + input.AccessToken)
// 	if err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể kết nối Facebook"})
// 		return
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode != http.StatusOK {
// 		c.JSON(http.StatusUnauthorized, gin.H{"error": "AccessToken không hợp lệ"})
// 		return
// 	}

// 	var fbData struct {
// 		ID    string `json:"id"`
// 		Name  string `json:"name"`
// 		Email string `json:"email"`
// 	}

// 	if err := json.NewDecoder(resp.Body).Decode(&fbData); err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không parse được dữ liệu Facebook"})
// 		return
// 	}

// 	// Nếu không có email thì có thể dùng fbData.ID để làm unique
// 	if fbData.Email == "" {
// 		fbData.Email = fbData.ID + "@facebook.com"
// 	}

// 	// Kiểm tra user trong DB
// 	var user models.User
// 	if err := config.DB.Where("email = ?", fbData.Email).First(&user).Error; err != nil {
// 		// Nếu chưa có thì tạo mới
// 		user = models.User{
// 			ID:       uuid.New().String(),
// 			Email:    fbData.Email,
// 			FullName:    fbData.Name,
// 			Role:   "user",
// 			Password:  "", // Facebook login nên không cần mật khẩu
// 		}
// 		if err := config.DB.Create(&user).Error; err != nil {
// 			c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo user Facebook"})
// 			return
// 		}
// 	}

// 	// Tạo JWT token
// 	token, err := utils.GenerateToken(user.ID, user.VaiTro)
// 	if err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo token"})
// 		return
// 	}

// 	c.JSON(http.StatusOK, gin.H{
// 		"token": token,
// 		"user": gin.H{
// 			"id":      user.ID,
// 			"email":   user.Email,
// 			"ho_ten":  user.HoTen,
// 			"vai_tro": user.VaiTro,
// 		},
// 	})
// }
