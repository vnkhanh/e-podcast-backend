package utils

import (
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTClaims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken tạo JWT token từ userID và role
func GenerateToken(userID string, role string) (string, error) {
	jwtKey := []byte(os.Getenv("JWT_SECRET")) // Đọc tại thời điểm gọi
	if len(jwtKey) == 0 {
		return "", errors.New("JWT_SECRET không được thiết lập")
	}

	claims := JWTClaims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKey)
}

// VerifyToken xác minh và parse JWT token
func VerifyToken(tokenStr string) (*JWTClaims, error) {
	jwtKey := []byte(os.Getenv("JWT_SECRET")) // Đọc tại thời điểm gọi
	if len(jwtKey) == 0 {
		return nil, errors.New("JWT_SECRET không được thiết lập")
	}

	token, err := jwt.ParseWithClaims(tokenStr, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("token không hợp lệ")
}
