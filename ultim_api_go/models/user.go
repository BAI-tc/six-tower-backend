package models

import (
	"time"
)

// User 对应 PostgreSQL 里的用户表 (users)
type User struct {
	UserID       int       `gorm:"column:user_id;primaryKey" json:"user_id"`
	Username     string    `gorm:"column:username;unique" json:"username"`
	Email        string    `gorm:"column:email;unique" json:"email"`
	PasswordHash string    `gorm:"column:password_hash" json:"-"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (User) TableName() string {
	return "users"
}
