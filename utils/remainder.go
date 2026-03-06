package utils

import (
	cron "github.com/robfig/cron/v3"
)

func StartReminderScheduler() {
	c := cron.New()

	// Run daily at 9 AM
	c.AddFunc("0 9 * * *", func() {
		// 1. Get upcoming birthdays/anniversaries (7 days ahead)
		// 2. Fetch salon-specific templates
		// 3. Send via Twilio API
		// 4. Log sent reminders
	})

	c.Start()
}
