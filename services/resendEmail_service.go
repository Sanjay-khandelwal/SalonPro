package services

import (
	"fmt"
	"salonpro-backend/config"
	"strings"

	"github.com/resend/resend-go/v2"
)

const (
	OTPExpiryMinutes         = 10
	OTPResendCooldownSeconds = 60
)

func CreateResentService(env *config.ENV) *resend.Client {
	// if environment or key missing return nil client, callers should check
	if env == nil || env.RESEND == nil || strings.TrimSpace(env.RESEND.RESEND_API_KEY) == "" {
		return nil
	}
	resendClient := resend.NewClient(env.RESEND.RESEND_API_KEY)
	if resendClient == nil {
		fmt.Println("failed to create resend connection")
	}
	return resendClient
}

type ResendService struct {
	Env          *config.ENV
	resendClient *resend.Client
}

// NewOTPService creates an OTP service. Resend client is nil if RESEND_API_KEY is empty.
func NewEmailService(env *config.ENV) *ResendService {
	// create client even if api key is empty; nil client will be handled during send
	resendClient := CreateResentService(env)
	return &ResendService{
		Env:          env,
		resendClient: resendClient,
	}
}

// SendOTPEmail sends the OTP to the given email via Resend. Returns error if client is nil or send fails.
func (s *ResendService) SendOTPOnEmail(toEmail string, otp string) error {
	if s == nil {
		return fmt.Errorf("resend service is nil")
	}
	if s.Env == nil || s.Env.RESEND == nil {
		return fmt.Errorf("resend service not configured with environment")
	}
	if s.resendClient == nil {
		return fmt.Errorf("resend client not initialized")
	}

	params := &resend.SendEmailRequest{
		From:    s.Env.RESEND.RESEND_FROM,
		To:      []string{strings.TrimSpace(toEmail)},
		Subject: "Password Reset OTP – SalonPro",
		Text:    fmt.Sprintf("Your password reset OTP is: %s. It expires in %d minutes. Do not share this code.", otp, OTPExpiryMinutes),
		Html:    fmt.Sprintf(`<h2>Password Reset</h2><p>Your OTP is: <strong>%s</strong></p><p>It expires in %d minutes. Do not share this code.</p>`, otp, OTPExpiryMinutes),
	}
	_, err := s.resendClient.Emails.Send(params)
	return err
}

func (s *ResendService) SendCustomerWelcomeEmail(toEmail string, customerName string) error {
	if s == nil || s.Env == nil || s.Env.RESEND == nil {
		return fmt.Errorf("resend service not configured")
	}
	if s.resendClient == nil {
		return fmt.Errorf("resend client not initialized")
	}

	htmlContent := fmt.Sprintf(`
<div style="font-family: Arial, sans-serif; background-color:#f9fafb; padding:20px;">
  <div style="max-width:600px; margin:0 auto; background:white; border-radius:8px; padding:30px;">
    
    <h2 style="color:#111827;">Welcome 🌟</h2>

    <p style="font-size:16px; color:#374151;">
      Hi <strong>%s</strong>,
    </p>

    <p style="font-size:15px; color:#4b5563; line-height:1.6;">
      We are delighted to welcome you to our salon family.
    </p>

    <p style="font-size:15px; color:#4b5563; line-height:1.6;">
      Thank you for choosing us. We look forward to providing you with a relaxing and wonderful experience every time you visit.
    </p>

    <p style="font-size:15px; color:#4b5563; margin-top:20px;">
      See you soon! ✨
    </p>

    <hr style="margin:30px 0; border:none; border-top:1px solid #e5e7eb;" />

    <p style="font-size:13px; color:#9ca3af;">
      Warm regards,<br/>
      The Salon Team
    </p>

  </div>
</div>
`, customerName)

	params := &resend.SendEmailRequest{
		From:    s.Env.RESEND.RESEND_FROM,
		To:      []string{strings.TrimSpace(toEmail)},
		Subject: "Welcome to SalonPro 🌟",
		Html:    htmlContent,
		Text: fmt.Sprintf(
			"Hi %s,\n\nThank you for being a valued customer of SalonPro.\nWe appreciate your trust in us.\n\n– The SalonPro Team",
			customerName,
		),
	}

	_, err := s.resendClient.Emails.Send(params)
	return err
}

// SendInvoiceEmail sends an invoice PDF as an email attachment to the customer.
// pdfBytes is the in-memory PDF (no disk file). If customer has no email, it returns nil (skipped).
func (s *ResendService) SendInvoiceEmail(toEmail, customerName, invoiceNumber, salonName string, pdfBytes []byte) error {
	if strings.TrimSpace(toEmail) == "" {
		return nil // customer has no email — skip silently
	}
	if s == nil || s.Env == nil || s.Env.RESEND == nil {
		return fmt.Errorf("resend service not configured")
	}
	if s.resendClient == nil {
		return fmt.Errorf("resend client not initialized")
	}

	htmlContent := fmt.Sprintf(`
<div style="font-family: Arial, sans-serif; background-color:#f9fafb; padding:20px;">
  <div style="max-width:600px; margin:0 auto; background:white; border-radius:8px; padding:30px;">
    <h2 style="color:#111827;">Your Invoice from %s</h2>
    <p style="font-size:16px; color:#374151;">Hi <strong>%s</strong>,</p>
    <p style="font-size:15px; color:#4b5563; line-height:1.6;">
      Thank you for visiting us! Please find your invoice <strong>%s</strong> attached to this email.
    </p>
    <p style="font-size:15px; color:#4b5563; line-height:1.6;">
      We look forward to seeing you again soon. ✨
    </p>
    <hr style="margin:30px 0; border:none; border-top:1px solid #e5e7eb;" />
    <p style="font-size:13px; color:#9ca3af;">
      Warm regards,<br/>%s Team
    </p>
  </div>
</div>`, salonName, customerName, invoiceNumber, salonName)

	filename := fmt.Sprintf("invoice-%s.pdf", invoiceNumber)

	params := &resend.SendEmailRequest{
		From:    s.Env.RESEND.RESEND_FROM,
		To:      []string{strings.TrimSpace(toEmail)},
		Subject: fmt.Sprintf("Your Invoice %s – %s", invoiceNumber, salonName),
		Html:    htmlContent,
		Text: fmt.Sprintf(
			"Hi %s,\n\nThank you for visiting %s! Please find your invoice %s attached.\n\nWarm regards,\n%s Team",
			customerName, salonName, invoiceNumber, salonName,
		),
		Attachments: []*resend.Attachment{
			{
				Filename: filename,
				Content:  pdfBytes,
			},
		},
	}
	_, err := s.resendClient.Emails.Send(params)
	return err
}

// SendReminderEmail sends a birthday or anniversary reminder email to a customer.
// eventType should be "birthday" or "anniversary".
func (s *ResendService) SendReminderEmail(toEmail, customerName, salonName, message, eventType string) error {
	if strings.TrimSpace(toEmail) == "" {
		return nil // customer has no email — skip silently
	}
	if s == nil || s.Env == nil || s.Env.RESEND == nil {
		return fmt.Errorf("resend service not configured")
	}
	if s.resendClient == nil {
		return fmt.Errorf("resend client not initialized")
	}

	var subject string
	var emoji string
	switch eventType {
	case "birthday":
		subject = fmt.Sprintf("Happy Birthday from %s! 🎂", salonName)
		emoji = "🎂"
	default:
		subject = fmt.Sprintf("Happy Anniversary from %s! 🎊", salonName)
		emoji = "🎊"
	}

	htmlContent := fmt.Sprintf(`
<div style="font-family: Arial, sans-serif; background-color:#f9fafb; padding:20px;">
  <div style="max-width:600px; margin:0 auto; background:white; border-radius:8px; padding:30px;">
    <h2 style="color:#111827;">%s %s</h2>
    <p style="font-size:16px; color:#374151;">Hi <strong>%s</strong>,</p>
    <p style="font-size:15px; color:#4b5563; line-height:1.6;">%s</p>
    <hr style="margin:30px 0; border:none; border-top:1px solid #e5e7eb;" />
    <p style="font-size:13px; color:#9ca3af;">
      Warm regards,<br/>%s Team
    </p>
  </div>
</div>`, subject, emoji, customerName, message, salonName)

	params := &resend.SendEmailRequest{
		From:    s.Env.RESEND.RESEND_FROM,
		To:      []string{strings.TrimSpace(toEmail)},
		Subject: subject,
		Html:    htmlContent,
		Text:    fmt.Sprintf("%s\n\n%s\n\n– %s Team", subject, message, salonName),
	}
	_, err := s.resendClient.Emails.Send(params)
	return err
}

func (s *ResendService) SendEmployeeNotification(name, email, password string) error {
	if s == nil || s.Env == nil || s.Env.RESEND == nil {
		return fmt.Errorf("resend service not configured")
	}
	if s.resendClient == nil {
		return fmt.Errorf("resend client not initialized")
	}

	htmlContent := fmt.Sprintf(`
	<div style="font-family: Arial, sans-serif; background-color:#f9fafb; padding:20px;">
		<div style="max-width:600px; margin:0 auto; background:white; border-radius:8px; padding:30px;">
			
			<h2 style="color:#111827;">You're Invited to Join the Salon Team 🎉</h2>

			<p style="font-size:16px; color:#374151;">
				Hi <strong>%s</strong>,
			</p>

			<p style="font-size:15px; color:#4b5563; line-height:1.6;">
				You have been added as an employee in our salon management system.
			</p>

			<p style="font-size:15px; color:#4b5563; line-height:1.6;">
				Here are your login credentials:
			</p>

			<div style="background:#f3f4f6; padding:15px; border-radius:6px; margin:15px 0;">
				<p style="margin:5px 0;"><strong>Email:</strong> %s</p>
				<p style="margin:5px 0;"><strong>Temporary Password:</strong> %s</p>
			</div>

			<p style="font-size:14px; color:#dc2626;">
				For security reasons, please change your password after your first login.
			</p>

			<hr style="margin:30px 0; border:none; border-top:1px solid #e5e7eb;" />

			<p style="font-size:13px; color:#9ca3af;">
				If you have any questions, please contact your salon administrator.
			</p>

			<p style="font-size:13px; color:#9ca3af;">
				Best regards,<br/>
				Salon Management Team
			</p>
		</div>
	</div>
	`, name, email, password)

	params := &resend.SendEmailRequest{
		From:    s.Env.RESEND.RESEND_FROM,
		To:      []string{strings.TrimSpace(email)},
		Subject: "Employee Invitation – Login Details Inside",
		Html:    htmlContent,
		Text: fmt.Sprintf(
			"Hi %s,\n\nYou have been added as an employee.\n\nEmail: %s\nTemporary Password: %s\n\nPlease change your password after first login.\n\nSalon Management Team",
			name, email, password,
		),
	}

	_, err := s.resendClient.Emails.Send(params)
	return err
}
