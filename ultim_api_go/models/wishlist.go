package models

import (
	"time"
)

// UserWishlist 对应 PostgreSQL 里的用户愿望单表 (user_wishlist)
type UserWishlist struct {
	ID            int        `gorm:"column:id;primaryKey" json:"id"`
	SteamID       string     `gorm:"column:steam_id;index" json:"steam_id"`
	GameID        int        `gorm:"column:game_id;index" json:"game_id"`
	GameName      string     `gorm:"column:game_name" json:"game_name"`
	GameData      string     `gorm:"column:game_data" json:"game_data"` // JSON 存储游戏详细信息
	AddedAt       time.Time  `gorm:"column:added_at" json:"added_at"`
	PriceWhenAdded *float64  `gorm:"column:price_when_added" json:"price_when_added"`
	CurrentPrice  *float64   `gorm:"column:current_price" json:"current_price"`
	CreatedAt     time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (UserWishlist) TableName() string {
	return "user_wishlist"
}
