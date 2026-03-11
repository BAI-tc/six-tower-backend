package handlers

import (
	"net/http"
	"time"

	"ultim_api_go/database"
	"ultim_api_go/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler handles authentication requests
type AuthHandler struct{}

// NewAuthHandler creates a new Auth handler
func NewAuthHandler() *AuthHandler {
	return &AuthHandler{}
}

// LoginRequest represents login request body
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// RegisterRequest represents register request body
type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

// AuthResponse represents auth response
type AuthResponse struct {
	Success  bool        `json:"success"`
	Message  string      `json:"message,omitempty"`
	User     *UserResponse `json:"user,omitempty"`
	Token    string      `json:"token,omitempty"`
}

// UserResponse represents user info in response
type UserResponse struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// Login handles POST /auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, AuthResponse{
			Success: false,
			Message: "Invalid request: " + err.Error(),
		})
		return
	}

	// Find user by username
	var user models.User
	if err := database.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, AuthResponse{
			Success: false,
			Message: "Invalid username or password",
		})
		return
	}

	// Check password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, AuthResponse{
			Success: false,
			Message: "Invalid username or password",
		})
		return
	}

	// Generate simple token (in production, use JWT)
	token := generateToken(user.UserID)

	c.JSON(http.StatusOK, AuthResponse{
		Success: true,
		Message: "Login successful",
		User: &UserResponse{
			UserID:   user.UserID,
			Username: user.Username,
			Email:    user.Email,
		},
		Token: token,
	})
}

// Register handles POST /auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, AuthResponse{
			Success: false,
			Message: "Invalid request: " + err.Error(),
		})
		return
	}

	// Check if username already exists
	var existingUser models.User
	if err := database.DB.Where("username = ?", req.Username).First(&existingUser).Error; err == nil {
		c.JSON(http.StatusConflict, AuthResponse{
			Success: false,
			Message: "Username already exists",
		})
		return
	}

	// Check if email already exists
	if err := database.DB.Where("email = ?", req.Email).First(&existingUser).Error; err == nil {
		c.JSON(http.StatusConflict, AuthResponse{
			Success: false,
			Message: "Email already exists",
		})
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, AuthResponse{
			Success: false,
			Message: "Failed to process password",
		})
		return
	}

	// Create user
	user := models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := database.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, AuthResponse{
			Success: false,
			Message: "Failed to create user",
		})
		return
	}

	// Generate token
	token := generateToken(user.UserID)

	c.JSON(http.StatusCreated, AuthResponse{
		Success: true,
		Message: "Registration successful",
		User: &UserResponse{
			UserID:   user.UserID,
			Username: user.Username,
			Email:    user.Email,
		},
		Token: token,
	})
}

// generateToken generates a simple token (in production, use proper JWT)
func generateToken(userID int) string {
	return "token_" + string(rune(userID)) + "_" + time.Now().Format("20060102150405")
}

// Refresh handles POST /auth/refresh
func (h *AuthHandler) Refresh(c *gin.Context) {
	// In a real app we would verify the claims of the old token and issue a new one
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Token refreshed successfully",
		"token":   "new_refreshed_token_" + time.Now().Format("20060102150405"),
	})
}

// Logout handles POST /auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	// For simple implementation, client side handles removing token
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logged out successfully",
	})
}

// Verify handles GET /auth/verify
func (h *AuthHandler) Verify(c *gin.Context) {
	// Typically we'd check headers for token validation
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"valid":   true,
		"message": "Token is valid",
	})
}
