package controllers

import (
	"net/http"
	"salonpro-backend/models"
	"salonpro-backend/utils"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type UpdateProfileInput struct {
	SalonName    string `json:"salonName"`
	SalonAddress string `json:"salonAddress"`
	Phone        string `json:"phone"`
	Email        string `json:"email"`
	// WorkingHours models.JSONB `json:"workingHours"` // or your working hours struct
}

func (h *HandlerFunc) GetProfile(c *gin.Context) {
	// Get user ID from context
	userIDRaw, exists := c.Get("userId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "User ID not found")
		return
	}
	userUUID, err := uuid.Parse(userIDRaw.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Invalid user ID format")
		return
	}

	// --- Fetch user ---
	var user models.User
	if err := h.DB.First(&user, "id = ?", userUUID).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "User not found")
		return
	}

	// --- Fetch salon profile using user's SalonID ---
	var salon models.Salon
	if err := h.DB.First(&salon, "id = ?", user.SalonID).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Salon not found")
		return
	}

	// --- Fetch reminder templates ---
	var reminderTemplates []models.ReminderTemplate
	if err := h.DB.Where("salon_id = ?", salon.ID).Find(&reminderTemplates).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to fetch reminder templates")
		return
	}

	// Extract messages
	var birthdayMessage, anniversaryMessage string
	for _, tmpl := range reminderTemplates {
		switch tmpl.Type {
		case "birthday":
			birthdayMessage = tmpl.Message
		case "anniversary":
			anniversaryMessage = tmpl.Message
		}
	}

	// --- Return combined response ---
	c.JSON(http.StatusOK, gin.H{
		"salonProfile": gin.H{
			"salonName":    salon.Name,
			"address":      salon.Address,
			"phone":        user.Phone,
			"email":        user.Email,
			"workingHours": salon.WorkingHours,
		},
		"messageTemplates": gin.H{
			"birthday":    birthdayMessage,
			"anniversary": anniversaryMessage,
		},
		"notifications": gin.H{
			"birthdayReminders":    salon.BirthdayReminders,
			"anniversaryReminders": salon.AnniversaryReminders,
			"emailNotifications":   salon.EmailNotifications,
			"smsNotifications":     salon.SMSNotifications,
		},
	})
}

// UpdateSalonProfileInput: salon only — name and address (no user fields).
type UpdateSalonProfileInput struct {
	SalonName string `json:"salonName"`
	Address   string `json:"salonAddress"`
}

// UpdateSalonProfile updates only the salon's name and address (salon side only; does not touch user).
func (h *HandlerFunc) UpdateSalonProfile(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found")
		return
	}
	salonUUID, err := uuid.Parse(salonID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid salon ID")
		return
	}

	var input UpdateSalonProfileInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}

	updates := map[string]interface{}{}
	if input.SalonName != "" {
		updates["name"] = strings.TrimSpace(input.SalonName)
	}
	if input.Address != "" {
		updates["address"] = strings.TrimSpace(input.Address)
	}
	if len(updates) == 0 {
		utils.RespondWithError(c, http.StatusBadRequest, "Provide at least salonName or salonAddress to update")
		return
	}

	if err := h.DB.Model(&models.Salon{}).
		Where("id = ?", salonUUID).
		Updates(updates).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update salon info")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Salon profile updated successfully"})
}

type UpdateWorkingHoursInput struct {
	WorkingHours models.JSONB `json:"workingHours"`
}

func (h *HandlerFunc) UpdateWorkingHours(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found")
		return
	}
	salonUUID, err := uuid.Parse(salonID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid salon ID")
		return
	}

	var input UpdateWorkingHoursInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}

	if err := h.DB.Model(&models.Salon{}).
		Where("id = ?", salonUUID).
		Update("working_hours", input.WorkingHours).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update working hours")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Working hours updated successfully"})
}

type UpdateTemplatesInput struct {
	BirthdayMessage    string `json:"birthday" form:"birthday" binding:"omitempty"`
	AnniversaryMessage string `json:"anniversary" form:"anniversary" binding:"omitempty"`
}

func (h *HandlerFunc) UpdateReminderTemplates(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found")
		return
	}
	salonUUID, err := uuid.Parse(salonID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid salon ID")
		return
	}

	var input UpdateTemplatesInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}

	updates := []struct {
		Type    string
		Message string
	}{
		{"birthday", input.BirthdayMessage},
		{"anniversary", input.AnniversaryMessage},
	}

	for _, u := range updates {
		if err := h.DB.Model(&models.ReminderTemplate{}).
			Where("salon_id = ? AND type = ?", salonUUID, u.Type).
			Update("message", u.Message).Error; err != nil {
			utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update "+u.Type+" template")
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Templates updated successfully"})
}

type UpdateNotificationsInput struct {
	BirthdayReminders    bool `json:"birthdayReminders"`
	AnniversaryReminders bool `json:"anniversaryReminders"`
	EmailNotifications   bool `json:"emailNotifications"`
	SMSNotifications     bool `json:"smsNotifications"`
}

func (h *HandlerFunc) UpdateNotifications(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found")
		return
	}
	salonUUID, err := uuid.Parse(salonID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid salon ID")
		return
	}

	var input UpdateNotificationsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}

	if err := h.DB.Model(&models.Salon{}).
		Where("id = ?", salonUUID).
		Updates(map[string]interface{}{
			"birthday_reminders":   input.BirthdayReminders,
			"anniversary_reminders": input.AnniversaryReminders,
			"email_notifications":  input.EmailNotifications,
			"sms_notifications":    input.SMSNotifications,
		}).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update notifications")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notification settings updated successfully"})
}

// TestNotificationInput is the body for sending a test reminder email.
