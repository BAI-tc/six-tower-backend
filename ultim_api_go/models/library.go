package models

import (
	"time"
)

// UserLibrary 对应 PostgreSQL 里的用户游戏库表 (user_library)
type UserLibrary struct {
	LibraryID            int        `gorm:"column:library_id;primaryKey" json:"library_id"`
	UserID               int64      `gorm:"column:user_id" json:"user_id"`
	ProductID            int        `gorm:"column:product_id" json:"product_id"`
	AppID                string     `gorm:"column:app_id" json:"app_id"` // Steam App ID
	IsFavorite           bool       `gorm:"column:is_favorite" json:"is_favorite"`
	IsInstalled          bool       `gorm:"column:is_installed" json:"is_installed"`
	PurchaseDate         *time.Time `gorm:"column:purchase_date" json:"purchase_date"`
	PurchasePrice        *float64   `gorm:"column:purchase_price" json:"purchase_price"`
	PlaytimeHours        *float64   `gorm:"column:playtime_hours" json:"playtime_hours"`
	LastPlayedAt         *time.Time `gorm:"column:last_played_at" json:"last_played_at"`
	AchievementProgress  *int       `gorm:"column:achievement_progress" json:"achievement_progress"`
	AchievementsUnlocked *int       `gorm:"column:achievements_unlocked" json:"achievements_unlocked"`
	AchievementsTotal    *int       `gorm:"column:achievements_total" json:"achievements_total"`
	CreatedAt            time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt            time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (UserLibrary) TableName() string {
	return "user_library"
}
