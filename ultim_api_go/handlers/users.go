package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"ultim_api_go/database"
	"ultim_api_go/models"

	"github.com/gin-gonic/gin"
)

type UsersHandler struct{}

func NewUsersHandler() *UsersHandler {
	return &UsersHandler{}
}

func (h *UsersHandler) GetProfile(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "user_id is required"})
		return
	}

	var profile models.UserProfile
	if err := database.DB.Where("user_id = ?", userID).First(&profile).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Profile not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": profile})
}

func (h *UsersHandler) UpdateProfile(c *gin.Context) {
	var req models.UserProfile
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request: " + err.Error()})
		return
	}

	var existing models.UserProfile
	if err := database.DB.Where("user_id = ?", req.UserID).First(&existing).Error; err != nil {
		// Create profile if missing
		req.CreatedAt = time.Now()
		req.UpdatedAt = time.Now()
		if err := database.DB.Create(&req).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to create profile"})
			return
		}
	} else {
		existing.Bio = req.Bio
		existing.Location = req.Location
		existing.Website = req.Website
		existing.UpdatedAt = time.Now()
		if err := database.DB.Save(&existing).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update profile"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Profile updated"})
}

func (h *UsersHandler) GetInteractions(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "user_id is required"})
		return
	}

	var interactions []models.UserInteraction
	if err := database.DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&interactions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch interactions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": interactions})
}

func (h *UsersHandler) GetPlayedGames(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "user_id is required"})
		return
	}

	var played []models.UserInteraction
	if err := database.DB.Where("user_id = ? AND play_hours > ?", userID, 0).Find(&played).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch played games"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": played})
}

func (h *UsersHandler) GetPreferences(c *gin.Context) {
	// Typically we get steam ID using user_id via some auth context but for mockup:
	steamID := c.Query("steam_id")
	if steamID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "steam_id is required"})
		return
	}

	redisKey := "user_genres:" + steamID
	val, err := database.RDB.Get(database.Ctx, redisKey).Result()

	var genres []string
	if err == nil {
		json.Unmarshal([]byte(val), &genres)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"preferred_genres": genres,
		},
	})
}

func (h *UsersHandler) DeleteAccount(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "user_id is required"})
		return
	}

	// For safety, just mocking complete deletion logic
	if err := database.DB.Where("user_id = ?", userID).Delete(&models.User{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to delete account"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Account deleted"})
}

func (h *UsersHandler) GetProfileComplete(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "user_id is required"})
		return
	}

	var user models.User
	var profile models.UserProfile

	database.DB.Where("user_id = ?", userID).First(&user)
	database.DB.Where("user_id = ?", userID).First(&profile)

	score := 0
	if user.Username != "" {
		score += 25
	}
	if user.Email != "" {
		score += 25
	}
	if profile.Bio != "" {
		score += 25
	}
	if profile.AvatarURL != "" {
		score += 25
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"completion_score": score,
			"is_complete":      score == 100,
		},
	})
}
