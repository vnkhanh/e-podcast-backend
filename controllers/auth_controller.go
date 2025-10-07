package controllers

import (
	"net/http"
	"os"

	"cloud.google.com/go/auth/credentials/idtoken"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

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

	// ✅ Xác minh token với đúng GOOGLE_CLIENT_ID
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
			// Password để trống vì login Google
		}
		if err := config.DB.Create(&user).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo user Google"})
			return
		}
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
