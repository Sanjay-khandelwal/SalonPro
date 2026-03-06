package services

import (
	"fmt"
	"log"
	"salonpro-backend/models"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

type ReminderService struct {
	db           *gorm.DB
	emailService *ResendService
	smsService   *TwilioService
}

func NewReminderService(db *gorm.DB, emailService *ResendService, smsService *TwilioService) *ReminderService {
	if emailService == nil || emailService.resendClient == nil {
		log.Println("ReminderService: email service not configured (RESEND_API_KEY missing). Reminder emails disabled.")
	} else {
		log.Println("ReminderService: email service ready.")
	}
	if smsService == nil || smsService.ENV == nil || smsService.ENV.TWILIO == nil ||
		strings.TrimSpace(smsService.ENV.TWILIO.TWILIO_ACCOUNT_SID) == "" {
		log.Println("ReminderService: SMS service not configured (TWILIO credentials missing). Reminder SMS disabled.")
	} else {
		log.Println("ReminderService: SMS service ready.")
	}
	return &ReminderService{
		db:           db,
		emailService: emailService,
		smsService:   smsService,
	}
}

func (s *ReminderService) StartScheduler() {
	emailReady := s.emailService != nil && s.emailService.resendClient != nil
	smsReady := s.smsService != nil && s.smsService.ENV != nil && s.smsService.ENV.TWILIO != nil &&
		strings.TrimSpace(s.smsService.ENV.TWILIO.TWILIO_ACCOUNT_SID) != ""
	if !emailReady && !smsReady {
		log.Println("Reminder scheduler not started: neither email nor SMS service is configured.")
		return
	}
	c := cron.New(cron.WithSeconds())                     // enable seconds-level scheduling
	_, _ = c.AddFunc("0 9 * * * *", s.SendDailyReminders) // Every day at 9 AM
	//_, _ = c.AddFunc("@every 5s", s.SendDailyReminders)   // Also every 5 seconds
	c.Start()
	s.SendDailyReminders() // Run once on server startup
	log.Println("Reminder scheduler started (runs daily at 9 AM,  and once on startup)")
}

func (s *ReminderService) SendDailyReminders() {
	emailReady := s.emailService != nil && s.emailService.resendClient != nil
	smsReady := s.smsService != nil && s.smsService.ENV != nil && s.smsService.ENV.TWILIO != nil &&
		strings.TrimSpace(s.smsService.ENV.TWILIO.TWILIO_ACCOUNT_SID) != ""
	if !emailReady && !smsReady {
		return
	}
	log.Println("Starting daily reminder processing...")

	var users []models.User
	if err := s.db.Find(&users, "is_active = ?", true).Error; err != nil {
		log.Printf("Failed to fetch active users: %v", err)
		return
	}

	// Process each salon once (by SalonID)
	seen := make(map[uuid.UUID]bool)
	for _, u := range users {
		if seen[u.SalonID] {
			continue
		}
		seen[u.SalonID] = true
		s.ProcessSalonReminders(u.SalonID)
	}

	log.Println("Daily reminder processing completed")
}

func (s *ReminderService) ProcessSalonReminders(salonID uuid.UUID) {
	var salon models.Salon
	if err := s.db.First(&salon, "id = ?", salonID).Error; err != nil {
		log.Printf("Salon %s: not found: %v", salonID, err)
		return
	}
	if !salon.EmailNotifications && !salon.SMSNotifications {
		log.Printf("Salon %s: no notification channel enabled (enable Email or SMS in profile); skipping reminders", salonID)
		return
	}

	if salon.BirthdayReminders {
		birthdayCustomers, err := s.getUpcomingCustomers(salonID, "birthday")
		if err != nil {
			log.Printf("Salon %s: failed to get birthday customers: %v", salonID, err)
		} else {
			s.sendReminderEmails(salonID, birthdayCustomers, "birthday", &salon)
		}
	}

	if salon.AnniversaryReminders {
		anniversaryCustomers, err := s.getUpcomingCustomers(salonID, "anniversary")
		if err != nil {
			log.Printf("Salon %s: failed to get anniversary customers: %v", salonID, err)
		} else {
			s.sendReminderEmails(salonID, anniversaryCustomers, "anniversary", &salon)
		}
	}
}

func (s *ReminderService) getUpcomingCustomers(salonID uuid.UUID, eventType string) ([]models.Customer, error) {
	now := time.Now()

	var field string
	switch eventType {
	case "birthday":
		field = "birthday"
	case "anniversary":
		field = "anniversary"
	default:
		return nil, fmt.Errorf("invalid event type: %s", eventType)
	}

	// Build (month, day) pairs for today through today+7 (next 7 days inclusive)
	type monthDay struct{ M, D int }
	var pairs []monthDay
	for d := 0; d <= 7; d++ {
		t := now.AddDate(0, 0, d)
		pairs = append(pairs, monthDay{int(t.Month()), t.Day()})
	}

	var placeholders []string
	var args []interface{}
	args = append(args, salonID)
	for _, p := range pairs {
		placeholders = append(placeholders, "(?, ?)")
		args = append(args, p.M, p.D)
	}
	inClause := strings.Join(placeholders, ", ")

	query := fmt.Sprintf(`
		SELECT * FROM customers
		WHERE salon_id = ?
		AND is_active = true
		AND %s IS NOT NULL
		AND (EXTRACT(MONTH FROM %s), EXTRACT(DAY FROM %s)) IN (%s)
	`, field, field, field, inClause)

	var customers []models.Customer
	err := s.db.Raw(query, args...).Scan(&customers).Error
	return customers, err
}

// sendReminderEmails dispatches email and SMS notifications for all customers concurrently.
// Each customer gets their own goroutine so slow or failing providers don't stall the rest.
func (s *ReminderService) sendReminderEmails(salonID uuid.UUID, customers []models.Customer, eventType string, salon *models.Salon) {
	var template models.ReminderTemplate
	if err := s.db.Where("salon_id = ? AND type = ? AND is_active = true", salonID, eventType).
		First(&template).Error; err != nil {
		log.Printf("Salon %s: no active template for %s: %v", salonID, eventType, err)
		return
	}

	var wg sync.WaitGroup

	for _, customer := range customers {
		customer := customer // capture loop variable
		message := strings.ReplaceAll(template.Message, "[CustomerName]", customer.Name)

		wg.Add(1)
		go func() {
			defer wg.Done()

			// Send email if enabled and customer has an email
			if salon.EmailNotifications {
				if strings.TrimSpace(customer.Email) == "" {
					log.Printf("Salon %s: skipping email for customer %s — no email address", salonID, customer.Name)
				} else {
					if err := s.emailService.SendReminderEmail(customer.Email, customer.Name, salon.Name, message, eventType); err != nil {
						log.Printf("Salon %s: failed to send %s reminder email to %s: %v", salonID, eventType, customer.Email, err)
					} else {
						log.Printf("Salon %s: %s reminder email sent to %s (%s)", salonID, eventType, customer.Name, customer.Email)
					}
				}
			}

			// Send SMS if enabled and customer has a phone number
			if salon.SMSNotifications {
				if strings.TrimSpace(customer.Phone) == "" {
					log.Printf("Salon %s: skipping SMS for customer %s — no phone number", salonID, customer.Name)
				} else {
					if err := s.smsService.SendReminderSMS(customer.Phone, message); err != nil {
						log.Printf("Salon %s: failed to send %s reminder SMS to %s: %v", salonID, eventType, customer.Phone, err)
					} else {
						log.Printf("Salon %s: %s reminder SMS sent to %s (%s)", salonID, eventType, customer.Name, customer.Phone)
					}
				}
			}
		}()
	}

	wg.Wait()
}
