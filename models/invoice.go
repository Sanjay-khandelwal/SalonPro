package models

import (
	"time"

	"github.com/google/uuid"
)

type PaymentMethod struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	Name      string    `gorm:"uniqueIndex;not null"`      // e.g., "Cash", "Card", "UPI"
	Type      string    `gorm:"type:varchar(50);not null"` // e.g., 'cash', 'card', 'online'
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`

	Invoices []Invoice `gorm:"foreignKey:PaymentMethodID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

// ----------------------
// Invoice Model
// ----------------------
type Invoice struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	SalonID         uuid.UUID `gorm:"type:uuid;index;not null"`
	CreatedByUserID uuid.UUID `gorm:"type:uuid;index;not null"`
	CreatedByUser   User      `gorm:"foreignKey:CreatedByUserID"` // who created the invoice

	InvoiceNumber string    `gorm:"uniqueIndex;not null"`
	CustomerID    uuid.UUID `gorm:"type:uuid;index;not null"`
	Customer      Customer  `gorm:"foreignKey:CustomerID"` // customer details

	PaymentMethodID uuid.UUID     `gorm:"type:uuid;index;not null"`
	PaymentMethod   PaymentMethod `gorm:"foreignKey:PaymentMethodID"` // payment method details

	InvoiceDate time.Time `gorm:"default:CURRENT_TIMESTAMP"`

	Subtotal float64 `gorm:"type:decimal(10,2);not null"`
	Discount float64 `gorm:"type:decimal(10,2);default:0.0"`
	Tax      float64 `gorm:"type:decimal(10,2);default:0.0"`
	Total    float64 `gorm:"type:decimal(10,2);not null"`

	PaymentStatus string  `gorm:"type:varchar(20);default:'unpaid'"`
	PaidAmount    float64 `gorm:"type:decimal(10,2);default:0.0"`
	Notes         string

	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`

	// One-to-Many: Invoice -> InvoiceItem
	Items []InvoiceItem `gorm:"foreignKey:InvoiceID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

type InvoiceItem struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	InvoiceID   uuid.UUID `gorm:"type:uuid;index;not null"`
	ServiceID   uuid.UUID `gorm:"type:uuid;index;not null"`
	ServiceName string    `gorm:"not null"`
	Quantity    int       `gorm:"default:1"`
	UnitPrice   float64   `gorm:"type:decimal(10,2);not null"`
	TotalPrice  float64   `gorm:"type:decimal(10,2);not null"`
}
