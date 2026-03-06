package config

import (
	"log"
	"os"
	"sync"

	"github.com/joho/godotenv"
)

type Twilio struct {
	TWILIO_ACCOUNT_SID  string
	TWILIO_AUTH_TOKEN   string
	TWILIO_PHONE_NUMBER string
}
type Resend struct {
	RESEND_API_KEY string
	RESEND_FROM    string
}

type ENV struct {
	DB_URL           string
	JWT_SECRATE      string
	JWT_EXPIRY_HOURS string
	SERVER_PORT      string
	TURNSTILE_SECRET string
	TWILIO           *Twilio
	RESEND           *Resend
}

var (
	cfg  *ENV      // Singleton instance of ENV
	once sync.Once // Ensures LoadENV is executed only once (thread-safe)
)

func LoadENV() *ENV {
	once.Do(func() {
		err := godotenv.Load()
		if err != nil {
			log.Println("⚠ No .env file found, using system environment variables")
		} else {
			log.Println(" Environment variables loaded successfully")
		}
		cfg = &ENV{
			DB_URL:           os.Getenv("DB_URL"),
			JWT_SECRATE:      os.Getenv("JWT_SECRATE"),
			JWT_EXPIRY_HOURS: os.Getenv("JWT_EXPIRY_HOURS"),
			SERVER_PORT:      os.Getenv("PORT"),
			TURNSTILE_SECRET: os.Getenv("TURNSTILE_SECRET"),
			TWILIO: &Twilio{
				TWILIO_ACCOUNT_SID:  os.Getenv("TWILIO_ACCOUNT_SID"),
				TWILIO_AUTH_TOKEN:   os.Getenv("TWILIO_AUTH_TOKEN"),
				TWILIO_PHONE_NUMBER: os.Getenv("TWILIO_PHONE_NUMBER"),
			},
			RESEND: &Resend{
				RESEND_API_KEY: os.Getenv("RESEND_API_KEY"),
				RESEND_FROM:    os.Getenv("RESEND_FROM"),
			},
		}
	})

	return cfg
}
