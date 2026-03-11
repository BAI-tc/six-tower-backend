package handlers

import (
	"net/http"
	"strconv"
	"time"

	"ultim_api_go/database"
	"ultim_api_go/models"

	"github.com/gin-gonic/gin"
)

type InteractionsHandler struct{}

func NewInteractionsHandler() *InteractionsHandler {
	return &InteractionsHandler{}
}

type InteractRequest struct {
	UserID    int     `json:"user_id" binding:"required"`
	ProductID int     `json:"product_id" binding:"required"`
	PlayHours float64 `json:"play_hours"`
}

func (h *InteractionsHandler) Interact(c *gin.Context) {
	var req InteractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request: " + err.Error()})
		return
	}

	interaction := models.UserInteraction{
		UserID:    req.UserID,
		ProductID: req.ProductID,
		Timestamp: time.Now().Unix(),
		PlayHours: req.PlayHours,
		CreatedAt: time.Now(),
	}

	if err := database.DB.Create(&interaction).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to record interaction"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Interaction recorded"})
}

type ReviewRequest struct {
	UserID     int     `json:"user_id" binding:"required"`
	ProductID  int     `json:"product_id" binding:"required"`
	Rating     float64 `json:"rating" binding:"required"`
	ReviewText string  `json:"review_text"`
}

func (h *InteractionsHandler) SubmitReview(c *gin.Context) {
	var req ReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request: " + err.Error()})
		return
	}

	review := models.UserReview{
		UserID:     req.UserID,
		ProductID:  req.ProductID,
		Rating:     req.Rating,
		ReviewText: req.ReviewText,
		CreatedAt:  time.Now(),
	}

	if err := database.DB.Create(&review).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to submit review"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Review submitted"})
}

func (h *InteractionsHandler) GetReviews(c *gin.Context) {
	productID := c.Param("product_id")
	limit := c.DefaultQuery("limit", "10")

	limitInt, _ := strconv.Atoi(limit)
	if limitInt <= 0 {
		limitInt = 10
	}

	var reviews []models.UserReview
	if err := database.DB.Where("product_id = ?", productID).Order("created_at DESC").Limit(limitInt).Find(&reviews).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch reviews"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": reviews})
}

type FeedbackRequest struct {
	UserID           int    `json:"user_id" binding:"required"`
	ProductID        int    `json:"product_id" binding:"required"`
	FeedbackType     string `json:"feedback_type" binding:"required"`
	RecommendationID string `json:"recommendation_id"`
}

func (h *InteractionsHandler) SubmitFeedback(c *gin.Context) {
	var req FeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request: " + err.Error()})
		return
	}

	feedback := models.UserFeedback{
		UserID:           req.UserID,
		ProductID:        req.ProductID,
		FeedbackType:     req.FeedbackType,
		RecommendationID: req.RecommendationID,
		CreatedAt:        time.Now(),
	}

	if err := database.DB.Create(&feedback).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to submit feedback"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Feedback submitted"})
}

func (h *InteractionsHandler) GetHistory(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "user_id is required"})
		return
	}

	var history []models.UserInteraction
	if err := database.DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&history).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": history})
}

func (h *InteractionsHandler) DeleteHistory(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "user_id is required"})
		return
	}

	if err := database.DB.Where("user_id = ?", userID).Delete(&models.UserInteraction{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to delete history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "History deleted"})
}

func (h *InteractionsHandler) GetStats(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "user_id is required"})
		return
	}

	var count int64
	database.DB.Model(&models.UserInteraction{}).Where("user_id = ?", userID).Count(&count)

	var reviewsCount int64
	database.DB.Model(&models.UserReview{}).Where("user_id = ?", userID).Count(&reviewsCount)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"total_interactions": count,
			"total_reviews":      reviewsCount,
		},
	})
}
