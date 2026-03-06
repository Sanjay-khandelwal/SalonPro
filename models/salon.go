package models

import (
	"github.com/google/uuid"
)

type Salon struct {
	ID                   uuid.UUID `gorm:"type:uuid;primary_key"`
	Name                 string    `gorm:"not null"`
	Address              string
	WorkingHours         JSONB `gorm:"type:jsonb;default:'{}'"`
	BirthdayReminders    bool  `gorm:"default:true"`
	AnniversaryReminders bool  `gorm:"default:true"`
	EmailNotifications   bool  `gorm:"default:false"`
	SMSNotifications     bool  `gorm:"default:false"`

	Users             []User             `gorm:"foreignKey:SalonID"`
	Customers         []Customer         `gorm:"foreignKey:SalonID"`
	Services          []Service          `gorm:"foreignKey:SalonID"`
	Invoices          []Invoice          `gorm:"foreignKey:SalonID"`
	ReminderTemplates []ReminderTemplate `gorm:"foreignKey:SalonID"`
}
