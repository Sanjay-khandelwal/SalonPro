package models

import (
	"github.com/google/uuid"
)

type ReminderTemplate struct {
	ID       uuid.UUID `gorm:"type:uuid;primary_key"`
	SalonID  uuid.UUID `gorm:"type:uuid;index;not null"`
	Type     string    `gorm:"type:reminder_type;not null"`
	Message  string    `gorm:"type:text;not null"`
	IsActive bool      `gorm:"default:true"`
}
