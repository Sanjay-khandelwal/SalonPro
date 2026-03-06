package config

import (
	"log"
	"strings"

	"salonpro-backend/models"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func ConnectDB(DB_URL string) *gorm.DB {
	dsn := strings.TrimSpace(DB_URL)
	if dsn == "" {
		panic("DB_URL environment variable is not set")
	}
	// Must be a postgres URL; if it starts with "psql" or has quotes, parsing will fail
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		panic("DB_URL must be a postgres URL starting with postgres:// or postgresql:// (no 'psql' command or extra quotes)")
	}

	// use silent logging to suppress GORM SQL output (including slow query logs)
	importLogger := logger.Default.LogMode(logger.Silent)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: importLogger})
	if err != nil {
		panic("Failed to connect database: " + err.Error())
	}

	// Enable uuid-ossp extension so uuid_generate_v4() exists for UUID defaults
	if err := db.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`).Error; err != nil {
		panic("Failed to create uuid-ossp extension: " + err.Error())
	}

	// Create reminder_type enum for reminder_templates (required before creating reminder_templates table)
	if err := db.Exec(`
		DO $$ BEGIN
			CREATE TYPE reminder_type AS ENUM ('birthday', 'anniversary');
		EXCEPTION
			WHEN duplicate_object THEN null;
		END $$;
	`).Error; err != nil {
		panic("Failed to create reminder_type enum: " + err.Error())
	}

	// Create payment_status enum type for invoices (required before creating invoices table)
	if err := db.Exec(`
		DO $$ BEGIN
			CREATE TYPE payment_status AS ENUM ('unpaid', 'paid', 'partial');
		EXCEPTION
			WHEN duplicate_object THEN null;
		END $$;
	`).Error; err != nil {
		panic("Failed to create payment_status enum: " + err.Error())
	}

	// OTP table: migrate from user_id to email (only if table already exists from a previous schema)
	db.Exec(`
		DO $$ BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'otps') THEN
				ALTER TABLE otps ADD COLUMN IF NOT EXISTS email VARCHAR(255);
				CREATE INDEX IF NOT EXISTS idx_otps_email ON otps(email);
				ALTER TABLE otps DROP COLUMN IF EXISTS user_id;
			END IF;
		END $$;
	`)
	return db
}

// SeedPaymentMethods inserts default payment methods if the table is empty, and backfills invoice.payment_method_id if needed.
func SeedPaymentMethods(DB *gorm.DB) {
	var count int64
	if err := DB.Model(&models.PaymentMethod{}).Count(&count).Error; err != nil {
		return
	}
	if count == 0 {
		defaults := []models.PaymentMethod{
			{ID: uuid.New(), Name: "Cash", Type: "cash"},
			{ID: uuid.New(), Name: "Card", Type: "card"},
			{ID: uuid.New(), Name: "UPI", Type: "online"},
			{ID: uuid.New(), Name: "Online", Type: "online"},
		}
		for _, pm := range defaults {
			if err := DB.Create(&pm).Error; err != nil {
				log.Printf("SeedPaymentMethods: skip %s: %v", pm.Name, err)
			}
		}
		log.Println("SeedPaymentMethods: default payment methods seeded")
	}
	// Backfill invoices that have nil/zero payment_method_id (e.g. after schema migration)
	var firstID uuid.UUID
	if err := DB.Model(&models.PaymentMethod{}).Select("id").Order("name").Limit(1).Scan(&firstID).Error; err != nil || firstID == uuid.Nil {
		return
	}
	res := DB.Exec("UPDATE invoices SET payment_method_id = ? WHERE payment_method_id IS NULL OR payment_method_id = ?", firstID, uuid.Nil)
	if res.RowsAffected > 0 {
		log.Printf("SeedPaymentMethods: backfilled payment_method_id for %d invoice(s)", res.RowsAffected)
	}
}

// MigrateInvoicesPaymentMethod adds payment_method_id to invoices if missing (run after AutoMigrate so payment_methods exists).
func MigrateInvoicesPaymentMethod(DB *gorm.DB) {
	if !DB.Migrator().HasTable("payment_methods") {
		return
	}
	if DB.Migrator().HasTable("invoices") && !DB.Migrator().HasColumn(&models.Invoice{}, "PaymentMethodID") {
		if err := DB.Migrator().AddColumn(&models.Invoice{}, "PaymentMethodID"); err != nil {
			log.Printf("MigrateInvoicesPaymentMethod: add column failed: %v", err)
			return
		}
		log.Println("MigrateInvoicesPaymentMethod: added payment_method_id to invoices")
	}
}

func AutoMigrationRun(DB *gorm.DB) {
	DB.AutoMigrate(
		&models.Salon{},
		&models.User{},
		&models.Customer{},
		&models.Service{},
		&models.PaymentMethod{}, // before Invoice (FK reference)
		&models.Invoice{},
		&models.InvoiceItem{},
		&models.ReminderTemplate{},
		&models.OTP{},
		//&models.ReminderLog{},
	)
	MigrateInvoicesPaymentMethod(DB) // add payment_method_id to invoices if old schema
	SeedPaymentMethods(DB)
}
