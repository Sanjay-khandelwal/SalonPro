package services

import (
	"fmt"
	"salonpro-backend/config"
	"strings"

	"github.com/twilio/twilio-go"
	api "github.com/twilio/twilio-go/rest/api/v2010"
)

func CrerateTwilioService(env *config.ENV) *twilio.RestClient {

	client := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: env.TWILIO.TWILIO_ACCOUNT_SID,
		Password: env.TWILIO.TWILIO_AUTH_TOKEN,
	})
	if client == nil {
		fmt.Println("Twilio client initialization failed: missing credentials")
	}
	return client
}

type TwilioService struct {
	ENV               *config.ENV
	TwiliowSMSService *twilio.RestClient
}

func NewSMSService(env *config.ENV) *TwilioService {
	return &TwilioService{
		ENV:               env,
		TwiliowSMSService: CrerateTwilioService(env),
	}
}

func ChackStatus(result *api.ApiV2010Message) {
	status := *result.Status
	switch status {

	case "queued":
		fmt.Println(" Message accepted and queued for sending")

	case "sending":
		fmt.Println(" Message is being sent")

	case "sent":
		fmt.Println(" Message sent to carrier")

	case "delivered":
		fmt.Println(" Message delivered successfully")

	case "failed":
		fmt.Println(" Message failed")
		if result.ErrorMessage != nil {
			fmt.Println("Reason:", *result.ErrorMessage)
		}

	case "undelivered":
		fmt.Println("⚠ Message undelivered")
		if result.ErrorMessage != nil {
			fmt.Println("Reason:", *result.ErrorMessage)
		}

	default:
		fmt.Println("ℹ Unknown Status:", status)
	}
}

// SendReminderSMS sends a birthday or anniversary reminder SMS to a customer.
// This is a synchronous (blocking) call — only call it from a background worker/scheduler, never from an HTTP handler.
func (s *TwilioService) SendReminderSMS(toPhone, message string) error {
	if s == nil || s.ENV == nil || s.ENV.TWILIO == nil {
		return fmt.Errorf("twilio service configuration is nil")
	}
	if strings.TrimSpace(s.ENV.TWILIO.TWILIO_ACCOUNT_SID) == "" || strings.TrimSpace(s.ENV.TWILIO.TWILIO_AUTH_TOKEN) == "" {
		return fmt.Errorf("twilio credentials not configured")
	}
	if strings.TrimSpace(s.ENV.TWILIO.TWILIO_PHONE_NUMBER) == "" {
		return fmt.Errorf("TWILIO_PHONE_NUMBER not set")
	}
	if strings.TrimSpace(toPhone) == "" {
		return fmt.Errorf("customer phone number is empty")
	}

	params := &api.CreateMessageParams{}
	params.SetTo(strings.TrimSpace(toPhone))
	params.SetFrom(s.ENV.TWILIO.TWILIO_PHONE_NUMBER)
	params.SetBody(message)

	result, err := s.TwiliowSMSService.Api.CreateMessage(params)
	if err != nil {
		return fmt.Errorf("failed to send SMS to %s: %w", toPhone, err)
	}
	ChackStatus(result)
	return nil
}

// SendCustomerWelcomeSMS fires a welcome SMS in the background and returns immediately.
// Errors are logged to stdout; they do not propagate to the caller.
func (s *TwilioService) SendCustomerWelcomeSMS(toPhone string, customerName string) error {
	if s == nil || s.ENV == nil || s.ENV.TWILIO == nil {
		return fmt.Errorf("twilio service configuration is nil")
	}
	if strings.TrimSpace(toPhone) == "" {
		return fmt.Errorf("customer phone number is empty")
	}

	messageBody := fmt.Sprintf("Hi %s, welcome to our salon!", customerName)

	params := &api.CreateMessageParams{}
	params.SetTo(strings.TrimSpace(toPhone))
	params.SetFrom(s.ENV.TWILIO.TWILIO_PHONE_NUMBER)
	params.SetBody(messageBody)

	result, sendErr := s.TwiliowSMSService.Api.CreateMessage(params)
	if sendErr != nil {
		fmt.Printf("SendCustomerWelcomeSMS: failed to send SMS to %s: %v\n", toPhone, sendErr)
	}
	ChackStatus(result)

	return nil
}
