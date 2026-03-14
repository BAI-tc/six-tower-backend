package models

import (
	"time"
)

// GameMetadata 对应 PostgreSQL 里的游戏基础信息表 (game_metadata)
type GameMetadata struct {
	ProductID        int       `gorm:"column:product_id;primaryKey" json:"product_id"`
	Title            string    `gorm:"column:title" json:"title"`
	AppName          string    `gorm:"column:app_name" json:"app_name"`
	Genres           string    `gorm:"column:genres" json:"genres"` // 逗号分隔
	Tags             string    `gorm:"column:tags" json:"tags"`     // 逗号分隔
	Developer        string    `gorm:"column:developer" json:"developer"`
	Publisher        string    `gorm:"column:publisher" json:"publisher"`
	Metascore        *int      `gorm:"column:metascore" json:"metascore"`
	Sentiment        string    `gorm:"column:sentiment" json:"sentiment"`
	ReleaseDate      string    `gorm:"column:release_date" json:"release_date"`
	Price            *float64  `gorm:"column:price" json:"price"`
	DiscountPrice    *float64  `gorm:"column:discount_price" json:"discount_price"`
	Description      string    `gorm:"column:description" json:"description"`
	ShortDescription string    `gorm:"column:short_description" json:"short_description"`
	Specs            string    `gorm:"column:specs" json:"specs"` // 逗号分隔
	BackgroundImage  string    `gorm:"column:background_image" json:"background_image"` // RAWG 图片
	URL              string    `gorm:"column:url" json:"url"`
	ReviewsURL       string    `gorm:"column:reviews_url" json:"reviews_url"`
	EarlyAccess      bool      `gorm:"column:early_access" json:"early_access"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (GameMetadata) TableName() string {
	return "game_metadata"
}
