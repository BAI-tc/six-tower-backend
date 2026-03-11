package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"ultim_api_go/database"
	"ultim_api_go/models"

	"github.com/gin-gonic/gin"
)

// LibraryHandler handles library and wishlist requests
type LibraryHandler struct{}

// NewLibraryHandler creates a new Library handler
func NewLibraryHandler() *LibraryHandler {
	return &LibraryHandler{}
}

// WishlistRequest represents wishlist add/remove request
type WishlistRequest struct {
	SteamID string `json:"steam_id" binding:"required"`
	GameID  int    `json:"game_id" binding:"required"`
	GameData map[string]interface{} `json:"game_data"`
}

// GameStatusResponse represents game status response
type GameStatusResponse struct {
	Success    bool `json:"success"`
	InWishlist bool `json:"in_wishlist"`
}

// GetWishlist handles GET /library/wishlist
func (h *LibraryHandler) GetWishlist(c *gin.Context) {
	steamID := c.Query("steam_id")
	if steamID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "steam_id is required"})
		return
	}

	var wishlist []models.UserWishlist
	if err := database.DB.Where("steam_id = ?", steamID).Order("added_at DESC").Find(&wishlist).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch wishlist"})
		return
	}

	// Parse game_data for each wishlist item
	var result []gin.H
	for _, item := range wishlist {
		var gameData map[string]interface{}
		if item.GameData != "" {
			json.Unmarshal([]byte(item.GameData), &gameData)
		}
		result = append(result, gin.H{
			"id":              item.ID,
			"steam_id":        item.SteamID,
			"game_id":         item.GameID,
			"game_name":       item.GameName,
			"game_data":       gameData,
			"added_at":        item.AddedAt,
			"price_when_added": item.PriceWhenAdded,
			"current_price":   item.CurrentPrice,
		})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// AddToWishlist handles POST /library/wishlist
func (h *LibraryHandler) AddToWishlist(c *gin.Context) {
	var req WishlistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request: " + err.Error()})
		return
	}

	// Check if already exists
	var existing models.UserWishlist
	if err := database.DB.Where("steam_id = ? AND game_id = ?", req.SteamID, req.GameID).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Game already in wishlist"})
		return
	}

	// Serialize game_data
	gameDataJSON, _ := json.Marshal(req.GameData)

	wishlist := models.UserWishlist{
		SteamID:  req.SteamID,
		GameID:   req.GameID,
		GameName: req.GameData["name"].(string),
		GameData: string(gameDataJSON),
		AddedAt:  database.GetDB().NowFunc(),
	}

	if err := database.DB.Create(&wishlist).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to add to wishlist"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Added to wishlist"})
}

// RemoveFromWishlist handles DELETE /library/wishlist
func (h *LibraryHandler) RemoveFromWishlist(c *gin.Context) {
	var req WishlistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request: " + err.Error()})
		return
	}

	if err := database.DB.Where("steam_id = ? AND game_id = ?", req.SteamID, req.GameID).Delete(&models.UserWishlist{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to remove from wishlist"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Removed from wishlist"})
}

// GetGameStatus handles GET /library/game-status
func (h *LibraryHandler) GetGameStatus(c *gin.Context) {
	steamID := c.Query("steam_id")
	gameIDStr := c.Query("game_id")

	if steamID == "" || gameIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "steam_id and game_id are required"})
		return
	}

	gameID, err := strconv.Atoi(gameIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid game_id"})
		return
	}

	var wishlist models.UserWishlist
	inWishlist := false
	if err := database.DB.Where("steam_id = ? AND game_id = ?", steamID, gameID).First(&wishlist).Error; err == nil {
		inWishlist = true
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"in_wishlist": inWishlist},
	})
}

// GetFavorites handles GET /library/favorites
func (h *LibraryHandler) GetFavorites(c *gin.Context) {
	userIDStr := c.Query("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "user_id is required"})
		return
	}

	userID, _ := strconv.Atoi(userIDStr)

	var favorites []models.UserLibrary
	if err := database.DB.Where("user_id = ? AND is_favorite = ?", userID, true).Find(&favorites).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch favorites"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": favorites})
}

// ToggleFavoriteRequest describes request body for toggling favorites
type ToggleFavoriteRequest struct {
	UserID    int `json:"user_id" binding:"required"`
	ProductID int `json:"product_id" binding:"required"`
}

// ToggleFavorite handles POST /library/toggle-favorite
func (h *LibraryHandler) ToggleFavorite(c *gin.Context) {
	var req ToggleFavoriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request: " + err.Error()})
		return
	}

	var lib models.UserLibrary
	if err := database.DB.Where("user_id = ? AND product_id = ?", req.UserID, req.ProductID).First(&lib).Error; err != nil {
		// Mock creation if it doesn't exist for now
		lib = models.UserLibrary{
			UserID:    req.UserID,
			ProductID: req.ProductID,
			IsFavorite: true,
		}
		if err := database.DB.Create(&lib).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to add to library"})
			return
		}
	} else {
		lib.IsFavorite = !lib.IsFavorite
		database.DB.Save(&lib)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"message":     "Toggled favorite status",
		"is_favorite": lib.IsFavorite,
	})
}

