package models

import (
	"time"

	"github.com/google/uuid"
)

type Customer struct {
	ID              uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	SalonID         uuid.UUID `gorm:"type:uuid;index;not null"`
	CreatedByUserID uuid.UUID `gorm:"type:uuid;index;not null"`

	Name        string `gorm:"not null"`
	Phone       string `gorm:"not null"`
	Email       string
	Birthday    *time.Time
	Anniversary *time.Time
	Notes       string
	TotalVisits int     `gorm:"default:0"`
	TotalSpent  float64 `gorm:"type:decimal(10,2);default:0.0"`
	LastVisit   *time.Time
	IsActive    bool `gorm:"default:true"`

	Invoices []Invoice `gorm:"foreignKey:CustomerID"`
}
