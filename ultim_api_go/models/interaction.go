package models

import (
	"time"
)

// UserInteraction 对应 PostgreSQL 里的用户交互表 (user_interactions)
type UserInteraction struct {
	InteractionID int       `gorm:"column:interaction_id;primaryKey" json:"interaction_id"`
	UserID        int       `gorm:"column:user_id" json:"user_id"`
	ProductID     int       `gorm:"column:product_id" json:"product_id"`
	Timestamp     int64     `gorm:"column:timestamp" json:"timestamp"`
	PlayHours     float64   `gorm:"column:play_hours" json:"play_hours"`
	EarlyAccess   bool      `gorm:"column:early_access" json:"early_access"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"created_at"`
}

func (UserInteraction) TableName() string {
	return "user_interactions"
}

// UserReview 对应 PostgreSQL 里的用户评价表 (user_reviews)
type UserReview struct {
	ReviewID   int       `gorm:"column:review_id;primaryKey" json:"review_id"`
	UserID     int       `gorm:"column:user_id" json:"user_id"`
	ProductID  int       `gorm:"column:product_id" json:"product_id"`
	Rating     float64   `gorm:"column:rating" json:"rating"`
	ReviewText string    `gorm:"column:review_text" json:"review_text"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
}

func (UserReview) TableName() string {
	return "user_reviews"
}

// UserFeedback 对应 PostgreSQL 里的用户反馈表 (user_feedback)
type UserFeedback struct {
	FeedbackID       int       `gorm:"column:feedback_id;primaryKey" json:"feedback_id"`
	UserID           int       `gorm:"column:user_id" json:"user_id"`
	ProductID        int       `gorm:"column:product_id" json:"product_id"`
	FeedbackType     string    `gorm:"column:feedback_type" json:"feedback_type"` // 'like', 'dislike', 'not_interested'
	RecommendationID string    `gorm:"column:recommendation_id" json:"recommendation_id"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"created_at"`
}

func (UserFeedback) TableName() string {
	return "user_feedback"
}

// RecommendationLog 对应 PostgreSQL 里的推荐日志表 (recommendation_logs)
type RecommendationLog struct {
	LogID            int       `gorm:"column:log_id;primaryKey"`
	UserID           int       `gorm:"column:user_id"`
	RecommendedItems string    `gorm:"column:recommended_items"` // JSON
	Algorithm        string    `gorm:"column:algorithm"`
	RecallTimeMs     *float64  `gorm:"column:recall_time_ms"`
	RankingTimeMs    *float64  `gorm:"column:ranking_time_ms"`
	TotalTimeMs      *float64  `gorm:"column:total_time_ms"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

func (RecommendationLog) TableName() string {
	return "recommendation_logs"
}
