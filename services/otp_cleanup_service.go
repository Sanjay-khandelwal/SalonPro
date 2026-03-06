package services

import (
	"fmt"
	"log"
	"salonpro-backend/models"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// OTPCleanupService runs a background cron job that deletes expired OTP records.
type OTPCleanupService struct {
	db      *gorm.DB
	cronJob *cron.Cron
}

// NewOTPCleanupService creates a new OTPCleanupService with the given DB connection.
func NewOTPCleanupService(db *gorm.DB) *OTPCleanupService {
	return &OTPCleanupService{db: db}
}

// Start registers the cleanup job to run every 20 seconds and starts the scheduler.
// It also runs one immediate cleanup on startup so stale OTPs don't linger after a restart.
func (s *OTPCleanupService) Start() {
	// Run one cleanup immediately on startup
	s.deleteExpired()

	// robfig/cron uses standard cron expressions (minimum 1-minute granularity).
	// For sub-minute intervals we use cron.Every() with a duration.
	s.cronJob = cron.New(cron.WithSeconds()) // enable seconds-level spec
	_, err := s.cronJob.AddFunc("@every 20s", func() {
		fmt.Println("otp clinup Every 20s")
		s.deleteExpired()
	})
	if err != nil {
		log.Printf("OTPCleanup: failed to register cron job: %v", err)
		return
	}

	s.cronJob.Start()
	log.Println("OTPCleanup: scheduler started — expired OTPs will be purged every 20 seconds")
}

// Stop gracefully stops the cron scheduler.
func (s *OTPCleanupService) Stop() {
	if s.cronJob != nil {
		s.cronJob.Stop()
		log.Println("OTPCleanup: scheduler stopped")
	}
}

// deleteExpired removes all OTP rows whose expires_at is before the current time.
func (s *OTPCleanupService) deleteExpired() {
	result := s.db.
		Where("expires_at < ?", time.Now()).
		Delete(&models.OTP{})

	if result.Error != nil {
		log.Printf("OTPCleanup: error deleting expired OTPs: %v", result.Error)
		return
	}
	if result.RowsAffected > 0 {
		log.Printf("OTPCleanup: deleted %d expired OTP(s)", result.RowsAffected)
	}
}
