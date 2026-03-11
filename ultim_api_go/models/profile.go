package models

import (
	"time"
)

// UserProfile 对应 PostgreSQL 里的用户画像扩展表 (user_profiles)
type UserProfile struct {
	ProfileID             int        `gorm:"column:profile_id;primaryKey" json:"profile_id"`
	UserID                int        `gorm:"column:user_id;unique" json:"user_id"`
	AvatarURL             string     `gorm:"column:avatar_url" json:"avatar_url"`
	Level                 *int       `gorm:"column:level" json:"level"`
	Exp                   *int       `gorm:"column:exp" json:"exp"`
	ExpToNextLevel        *int       `gorm:"column:exp_to_next_level" json:"exp_to_next_level"`
	MemberSince           *time.Time `gorm:"column:member_since" json:"member_since"`
	GamerDNAStats         string     `gorm:"column:gamer_dna_stats" json:"gamer_dna_stats"` // JSON
	PrimaryType           string     `gorm:"column:primary_type" json:"primary_type"`
	SecondaryType         string     `gorm:"column:secondary_type" json:"secondary_type"`
	TotalPlaytimeHours    *float64   `gorm:"column:total_playtime_hours" json:"total_playtime_hours"`
	GamesOwned            *int       `gorm:"column:games_owned" json:"games_owned"`
	LibraryValue          *float64   `gorm:"column:library_value" json:"library_value"`
	AchievementsUnlocked  *int       `gorm:"column:achievements_unlocked" json:"achievements_unlocked"`
	PerfectGames          *int       `gorm:"column:perfect_games" json:"perfect_games"`
	AvgSessionMinutes     *int       `gorm:"column:avg_session_minutes" json:"avg_session_minutes"`
	FavoriteGenres        string     `gorm:"column:favorite_genres" json:"favorite_genres"` // JSON
	CreatedAt             time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt             time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (UserProfile) TableName() string {
	return "user_profiles"
}
