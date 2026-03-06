package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"salonpro-backend/models"
	"salonpro-backend/services"
	"salonpro-backend/utils"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Role string

const (
	RoleOwner    Role = "owner"
	RoleManager  Role = "manager"
	RoleEmployee Role = "employee"
)

type RegisterInput struct {
	Email        string       `json:"email" binding:"required,email"`
	Phone        string       `json:"phone" binding:"required"`
	Name         string       `json:"name" binding:"required"`
	Password     string       `json:"password" binding:"required,min=8"`
	SalonName    string       `json:"salonName" binding:"required"`
	SalonAddress string       `json:"salonAddress"`
	WorkingHours models.JSONB `json:"workingHours"`
	CaptchaToken string       `json:"captchaToken" binding:"required"` // New field for CAPTCHA token
}

type LoginInput struct {
	Identifier   string `json:"identifier" binding:"required"` // Can be email or phone
	Password     string `json:"password" binding:"required"`
	CaptchaToken string `json:"captchaToken" binding:"required"` // New field for CAPTCHA token
}

type AddEmployeeInput struct {
	Email    string `json:"email" binding:"required,email"`
	Phone    string `json:"phone" binding:"required"`
	Name     string `json:"name" binding:"required"`
	Password string `json:"password" binding:"required,min=8"`
	Role     string `json:"role" binding:"required,oneof=manager employee"`
}

func (h *HandlerFunc) RegisterOTP(c *gin.Context) {
	var input RegisterInput

	// Bind and validate input
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}
	if !h.Service.VerifyTurnstile(input.CaptchaToken) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid capture"})
		return
	}
	// Check if email or phone already exists

	var existingUser models.User
	result := h.DB.Where("email = ? OR phone = ?", input.Email, input.Phone).First(&existingUser)

	if result.Error == nil {
		utils.RespondWithError(c, http.StatusConflict, "Email or phone already registered")
		return
	} else if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		return
	}
	otp, err := h.Service.GenerateOTP()
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to generate OTP")
		return
	}

	// Invalidate any existing OTPs for this email
	h.DB.Where("email = ?", input.Email).Delete(&models.OTP{})

	expiresAt := time.Now().Add(time.Duration(services.OTPExpiryMinutes) * time.Minute)
	otpRecord := models.OTP{
		Email:     input.Email,
		OTP:       otp,
		ExpiresAt: expiresAt,
	}
	if err := h.DB.Create(&otpRecord).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to store OTP")
		return
	}

	emailCopy := input.Email
	otpCopy := otp
	go func() {
		if err := h.Service.ResendService.SendOTPOnEmail(emailCopy, otpCopy); err != nil {
			fmt.Printf("RegisterOTP: failed to send OTP email to %s: %v\n", emailCopy, err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "Send OTP successfully"})

}

// Register - Creates salon owner account
func (h *HandlerFunc) Register(c *gin.Context) {
	var input RegisterInput

	// Bind and validate input
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}

	// if !h.Service.VerifyTurnstile(input.CaptchaToken) {
	// 	c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid capture"})
	// 	return
	// }
	// Check if email or phone already exists
	var existingUser models.User
	result := h.DB.Where("email = ? OR phone = ?", input.Email, input.Phone).First(&existingUser)
	fmt.Println("result", result)
	fmt.Println("err", result.Error)

	if result.Error == nil {
		utils.RespondWithError(c, http.StatusConflict, "Email or phone already registered")
		return
	} else if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		return
	}

	// Start transaction
	tx := h.DB.Begin()

	// Create salon first
	salon := models.Salon{
		ID:      uuid.New(),
		Name:    input.SalonName,
		Address: input.SalonAddress,
	}

	// Set default working hours if not provided
	if input.WorkingHours == nil {
		salon.WorkingHours = models.JSONB{
			"monday":    map[string]interface{}{"open": "09:00", "close": "20:00", "closed": false},
			"tuesday":   map[string]interface{}{"open": "09:00", "close": "20:00", "closed": false},
			"wednesday": map[string]interface{}{"open": "09:00", "close": "20:00", "closed": false},
			"thursday":  map[string]interface{}{"open": "09:00", "close": "20:00", "closed": false},
			"friday":    map[string]interface{}{"open": "09:00", "close": "20:00", "closed": false},
			"saturday":  map[string]interface{}{"open": "09:00", "close": "21:00", "closed": false},
			"sunday":    map[string]interface{}{"open": "10:00", "close": "19:00", "closed": true},
		}
	} else {
		salon.WorkingHours = input.WorkingHours
	}

	// Create salon
	if err := tx.Create(&salon).Error; err != nil {
		tx.Rollback()
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to create salon")
		return
	}

	// Create owner user
	newUser := models.User{
		ID:       uuid.New(),
		Email:    input.Email,
		Phone:    input.Phone,
		Name:     input.Name,
		Password: input.Password, // Will be hashed in BeforeCreate hook
		Role:     string(RoleOwner),
		SalonID:  salon.ID,
	}

	// Create user
	if err := tx.Create(&newUser).Error; err != nil {
		tx.Rollback()
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to create user")
		return
	}

	// Create default reminder templates
	if err := createDefaultReminderTemplates(tx, salon.ID); err != nil {
		tx.Rollback()
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to create reminder templates: "+err.Error())
		return
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Transaction commit failed")
		return
	}

	// Generate token
	token, err := utils.GenerateToken(newUser.ID.String(), salon.ID.String())
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to generate token")
		return
	}
	expiryHours := 24
	maxAge := expiryHours * 3600

	c.SetCookie(
		"token",
		token,
		maxAge,
		"/",
		"",
		true,
		true,
	)

	// Return response without password
	c.JSON(http.StatusCreated, gin.H{
		"message": "Registration successful",
		"token":   token,
		"user": gin.H{
			"id":    newUser.ID,
			"email": newUser.Email,
			"phone": newUser.Phone,
			"name":  newUser.Name,
			"role":  newUser.Role,
		},
		"salon": gin.H{
			"id":      salon.ID,
			"name":    salon.Name,
			"address": salon.Address,
		},
	})
}

// Login - Handles login for all user types
func (h *HandlerFunc) Login(c *gin.Context) {
	var input LoginInput

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	// Clean identifier
	identifier := strings.TrimSpace(input.Identifier)

	if !h.Service.VerifyTurnstile(input.CaptchaToken) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid capture"})
		return
	}

	// Find user by email or phone
	var user models.User
	query := h.DB.Where("email = ? OR phone = ?", identifier, identifier)
	result := query.First(&user)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			utils.RespondWithError(c, http.StatusUnauthorized, "Invalid credentials")
		} else {
			utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		}
		return
	}

	// Check if user is active
	if !user.IsActive {
		utils.RespondWithError(c, http.StatusUnauthorized, "Account is deactivated")
		return
	}

	// Check password
	if !utils.CheckPasswordHash(input.Password, user.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Get salon information
	var salon models.Salon
	if err := h.DB.First(&salon, "id = ?", user.SalonID).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Salon not found")
		return
	}

	// Generate token
	token, err := utils.GenerateToken(user.ID.String(), user.SalonID.String())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	// Update last login
	now := time.Now()
	h.DB.Model(&user).Update("last_login", &now)

	expiryHours := 24
	maxAge := expiryHours * 3600

	c.SetCookie(
		"token",
		token,
		maxAge,
		"/",
		"",
		true,
		true,
	)

	// Return response
	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user": gin.H{
			"id":    user.ID,
			"email": user.Email,
			"phone": user.Phone,
			"name":  user.Name,
			"role":  user.Role,
		},
		"salon": gin.H{
			"id":      salon.ID,
			"name":    salon.Name,
			"address": salon.Address,
		},
	})
}

// AddEmployee - Allows salon owner to add employees
func (h *HandlerFunc) AddEmployee(c *gin.Context) {
	var input AddEmployeeInput

	// Bind and validate input
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}
	fmt.Println(input)
	// Get current user from context
	userID, exists := c.Get("userId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	// Check if current user is owner or manager
	var currentUser models.User
	if err := h.DB.First(&currentUser, "id = ?", userID).Error; err != nil {
		utils.RespondWithError(c, http.StatusUnauthorized, "User not found")
		return
	}

	if currentUser.Role != string(RoleOwner) && currentUser.Role != string(RoleManager) {
		utils.RespondWithError(c, http.StatusForbidden, "Only owners and managers can add employees")
		return
	}

	// Check if email or phone already exists
	var existingUser models.User
	result := h.DB.Where("email = ? OR phone = ?", input.Email, input.Phone).First(&existingUser)

	if result.Error == nil {
		utils.RespondWithError(c, http.StatusConflict, "Email or phone already registered")
		return
	} else if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		return
	}

	salonIDStr, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon not found")
		return
	}

	salonUUID, err := uuid.Parse(salonIDStr.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusUnauthorized, "Invalid salon ID format")
		return
	}

	// Create new employee
	newEmployee := models.User{
		ID:       uuid.New(),
		Email:    input.Email,
		Phone:    input.Phone,
		Name:     input.Name,
		Password: input.Password, // Will be hashed in BeforeCreate hook
		Role:     input.Role,
		SalonID:  salonUUID,
	}

	// Create employee
	if err := h.DB.Create(&newEmployee).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to create employee")
		return
	}

	empName, empEmail, empPass := input.Name, input.Email, input.Password
	go func() {
		if err := h.Service.ResendService.SendEmployeeNotification(empName, empEmail, empPass); err != nil {
			fmt.Printf("AddEmployee: failed to send notification email to %s: %v\n", empEmail, err)
		}
	}()

	// Return response without password
	c.JSON(http.StatusCreated, gin.H{
		"message": "Employee added successfully",
		"employee": gin.H{
			"id":    newEmployee.ID,
			"email": newEmployee.Email,
			"phone": newEmployee.Phone,
			"name":  newEmployee.Name,
			"role":  newEmployee.Role,
		},
	})
}

// GetEmployees - Get all employees for a salon
func (h *HandlerFunc) GetEmployees(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon not found")
		return
	}

	var employees []models.User
	if err := h.DB.Where("salon_id = ? AND role != ?", salonID, string(RoleOwner)).Find(&employees).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to fetch employees")
		return
	}

	// Prepare response without passwords
	var employeeList []gin.H
	for _, emp := range employees {
		employeeList = append(employeeList, gin.H{
			"id":        emp.ID,
			"email":     emp.Email,
			"phone":     emp.Phone,
			"name":      emp.Name,
			"role":      emp.Role,
			"isActive":  emp.IsActive,
			"lastLogin": emp.LastLogin,
			"createdAt": emp.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"employees": employeeList,
	})
}

// UpdateEmployee - Update employee details
func (h *HandlerFunc) UpdateEmployee(c *gin.Context) {
	employeeID := c.Param("id")

	// Get current user from context
	userID, exists := c.Get("userId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon not found")
		return
	}

	// Check if current user is owner or manager
	var currentUser models.User
	if err := h.DB.First(&currentUser, "id = ?", userID).Error; err != nil {
		utils.RespondWithError(c, http.StatusUnauthorized, "User not found")
		return
	}

	if currentUser.Role != string(RoleOwner) && currentUser.Role != string(RoleManager) {
		utils.RespondWithError(c, http.StatusForbidden, "Only owners and managers can update employees")
		return
	}

	// Find employee
	var employee models.User
	if err := h.DB.Where("id = ? AND salon_id = ?", employeeID, salonID).First(&employee).Error; err != nil {
		utils.RespondWithError(c, http.StatusNotFound, "Employee not found")
		return
	}

	// Bind update data
	var updateData struct {
		Name     string `json:"name"`
		Phone    string `json:"phone"`
		Role     string `json:"role"`
		IsActive *bool  `json:"isActive"`
	}

	if err := c.ShouldBindJSON(&updateData); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}

	// Update employee
	updates := map[string]interface{}{}
	if updateData.Name != "" {
		updates["name"] = updateData.Name
	}
	if updateData.Phone != "" {
		updates["phone"] = updateData.Phone
	}
	if updateData.Role != "" && (updateData.Role == string(RoleManager) || updateData.Role == string(RoleEmployee)) {
		updates["role"] = updateData.Role
	}
	if updateData.IsActive != nil {
		updates["is_active"] = *updateData.IsActive
	}

	if err := h.DB.Model(&employee).Updates(updates).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update employee")
		return
	}

	// Return updated employee
	c.JSON(http.StatusOK, gin.H{
		"message": "Employee updated successfully",
		"employee": gin.H{
			"id":       employee.ID,
			"email":    employee.Email,
			"phone":    employee.Phone,
			"name":     employee.Name,
			"role":     employee.Role,
			"isActive": employee.IsActive,
		},
	})
}

// DeleteEmployee - Deactivate employee
func (h *HandlerFunc) DeleteEmployee(c *gin.Context) {
	employeeID := c.Param("id")

	// Get current user from context
	userID, exists := c.Get("userId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon not found")
		return
	}

	// Check if current user is owner
	var currentUser models.User
	if err := h.DB.First(&currentUser, "id = ?", userID).Error; err != nil {
		utils.RespondWithError(c, http.StatusUnauthorized, "User not found")
		return
	}

	if currentUser.Role != string(RoleOwner) {
		utils.RespondWithError(c, http.StatusForbidden, "Only owners can delete employees")
		return
	}

	// Find employee
	var employee models.User
	if err := h.DB.Where("id = ? AND salon_id = ?", employeeID, salonID).First(&employee).Error; err != nil {
		utils.RespondWithError(c, http.StatusNotFound, "Employee not found")
		return
	}

	// Don't allow deleting the owner
	if employee.Role == string(RoleOwner) {
		utils.RespondWithError(c, http.StatusForbidden, "Cannot delete salon owner")
		return
	}

	// Deactivate employee instead of deleting
	if err := h.DB.Model(&employee).Update("is_active", false).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to deactivate employee")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Employee deactivated successfully"})
}

// Me - Get current user information
func (h *HandlerFunc) Me(c *gin.Context) {
	userID, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	var user models.User
	if err := h.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found"})
		return
	}

	// Get salon information
	var salon models.Salon
	if err := h.DB.First(&salon, "id = ?", user.SalonID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Salon not found"})
		return
	}

	// Return user info
	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":    user.ID,
			"email": user.Email,
			"name":  user.Name,
			"phone": user.Phone,
			"role":  user.Role,
		},
		"salon": gin.H{
			"id":      salon.ID,
			"name":    salon.Name,
			"address": salon.Address,
		},
	})
}

// UpdateUserProfile updates the logged-in user's name, email, and phone only (user side, not salon).
func (h *HandlerFunc) UpdateUserProfile(c *gin.Context) {
	userID, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	userIDStr := userID.(string)
	userUUID := uuid.Must(uuid.Parse(userIDStr))
	var input models.UserProfile
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}
	if !utils.ValidatePhone(input.Phone) {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid phone number format")
		return
	}

	result := h.DB.Model(&models.User{}).
		Where("id = ?", userUUID).
		Updates(map[string]interface{}{
			"name":  strings.TrimSpace(input.Name),
			"phone": strings.TrimSpace(input.Phone),
		})

	if result.Error != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update profile")
		return
	}
	if result.RowsAffected == 0 {
		utils.RespondWithError(c, http.StatusNotFound, "User not found")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Profile updated successfully"})
}

// Helper function to create default reminder templates
func createDefaultReminderTemplates(tx *gorm.DB, salonID uuid.UUID) error {
	defaultTemplates := []models.ReminderTemplate{
		{
			ID:       uuid.New(),
			SalonID:  salonID,
			Type:     "birthday",
			Message:  "Hi [CustomerName], SalonPro wishes you a very happy birthday! 🎉 Enjoy 20% off on your next visit this month!",
			IsActive: true,
		},
		{
			ID:       uuid.New(),
			SalonID:  salonID,
			Type:     "anniversary",
			Message:  "Hi [CustomerName], happy salon anniversary! 🎊 Thank you for being our valued customer. Here's 15% off your next service!",
			IsActive: true,
		},
	}

	for _, tmpl := range defaultTemplates {
		// Check if this type already exists for the salon
		var existing models.ReminderTemplate
		err := tx.Where("salon_id = ? AND type = ?", salonID, tmpl.Type).First(&existing).Error
		if err == nil {
			continue // Template exists, skip
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("database error checking templates: %w", err)
		}

		if err := tx.Create(&tmpl).Error; err != nil {
			return fmt.Errorf("failed to create template: %w", err)
		}
	}
	return nil
}

// --- Forgot password (send OTP) ---

type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

func (h *HandlerFunc) ForgotPassword(c *gin.Context) {
	var input ForgotPasswordRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input")
		return
	}
	email := strings.TrimSpace(strings.ToLower(input.Email))

	var user models.User
	if err := h.DB.Where("email = ?", email).First(&user).Error; err != nil {
		// Do not reveal whether user exists
		c.JSON(http.StatusBadRequest, gin.H{"message": "email does not exists"})
		return
	}
	otp, err := h.Service.GenerateOTP()
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to generate OTP")
		return
	}

	// Invalidate any existing OTPs for this email
	h.DB.Where("email = ?", email).Delete(&models.OTP{})

	expiresAt := time.Now().Add(time.Duration(services.OTPExpiryMinutes) * time.Minute)
	otpRecord := models.OTP{
		Email:     email,
		OTP:       otp,
		ExpiresAt: expiresAt,
	}
	if err := h.DB.Create(&otpRecord).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to store OTP")
		return
	}

	fpEmail, fpOTP := user.Email, otp
	go func() {
		if err := h.Service.ResendService.SendOTPOnEmail(fpEmail, fpOTP); err != nil {
			fmt.Printf("ForgotPassword: failed to send OTP email to %s: %v\n", fpEmail, err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "If this email is registered, you will receive an OTP shortly."})
}

// --- Resend OTP (rate limited) ---

type ResendOTPRequest struct {
	Email string `json:"email" binding:"required,email"`
}

func (h *HandlerFunc) ResendOTP(c *gin.Context) {
	var input ResendOTPRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input")
		return
	}
	email := strings.TrimSpace(strings.ToLower(input.Email))

	// Rate limit: last OTP for this email within cooldown window
	var lastOTP models.OTP
	if err := h.DB.Where("email = ?", email).Order("created_at DESC").First(&lastOTP).Error; err == nil {
		if time.Since(lastOTP.CreatedAt) < services.OTPResendCooldownSeconds*time.Second {
			utils.RespondWithError(c, http.StatusTooManyRequests, "Please wait before requesting another OTP")
			return
		}
	}

	otp, err := h.Service.GenerateOTP()
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to generate OTP")
		return
	}

	h.DB.Where("email = ?", email).Delete(&models.OTP{})
	expiresAt := time.Now().Add(time.Duration(services.OTPExpiryMinutes) * time.Minute)
	otpRecord := models.OTP{Email: email, OTP: otp, ExpiresAt: expiresAt}
	if err := h.DB.Create(&otpRecord).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to store OTP")
		return
	}

	resendEmail, resendOTP := email, otp
	go func() {
		if err := h.Service.ResendService.SendOTPOnEmail(resendEmail, resendOTP); err != nil {
			fmt.Printf("ResendOTP: failed to send OTP email to %s: %v\n", resendEmail, err)
		}
	}()
	c.JSON(http.StatusOK, gin.H{"message": "otp send successfully"})
}

// --- Verify OTP (so frontend can then show change-password screen) ---

type VerifyOTPRequest struct {
	Email string `json:"email" binding:"required,email"`
	OTP   string `json:"otp" binding:"required"`
}

// VerifyOTP checks email + OTP. If valid, frontend can redirect to change-password form for that email.
func (h *HandlerFunc) VerifyOTP(c *gin.Context) {
	var input VerifyOTPRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Email and OTP are required")
		return
	}
	email := strings.TrimSpace(strings.ToLower(input.Email))
	otpCode := strings.TrimSpace(input.OTP)

	var otpRecord models.OTP
	if err := h.DB.Where("email = ? AND otp = ? AND expires_at > ?", email, otpCode, time.Now()).First(&otpRecord).Error; err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid or expired OTP")
		return
	}

	// OTP is valid; do not delete it yet — reset-password will consume it
	c.JSON(http.StatusOK, gin.H{
		"message":  "OTP verified. You can now set a new password.",
		"verified": true,
		"email":    email,
	})
}

// --- Reset password (after verify: user enters new password) ---

type ResetPasswordRequest struct {
	Email       string `json:"email" binding:"required,email"`
	OTP         string `json:"otp" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=8"`
}

func (h *HandlerFunc) ResetPassword(c *gin.Context) {
	var input ResetPasswordRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: email, otp, and newPassword (min 8 chars) required")
		return
	}
	email := strings.TrimSpace(strings.ToLower(input.Email))
	otpCode := strings.TrimSpace(input.OTP)

	var otpRecord models.OTP
	if err := h.DB.Where("email = ? AND otp = ? AND expires_at > ?", email, otpCode, time.Now()).First(&otpRecord).Error; err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid or expired OTP")
		return
	}

	var user models.User
	if err := h.DB.Where("email = ?", email).First(&user).Error; err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "User not found")
		return
	}

	hashed, err := utils.HashPassword(input.NewPassword)
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update password")
		return
	}
	if err := h.DB.Model(&user).Update("password", hashed).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update password")
		return
	}

	// Invalidate this OTP so it cannot be reused
	h.DB.Delete(&otpRecord)

	c.JSON(http.StatusOK, gin.H{"message": "Password reset successfully. You can now log in with your new password."})
}

func (h *HandlerFunc) ChangePassword(c *gin.Context) {
	type ChangePasswordRequest struct {
		CurrentPassword string `json:"currentPassword" binding:"required"`
		NewPassword     string `json:"newPassword" binding:"required,min=8"`
	}

	userIDStr, exists := c.Get("userId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "User ID not found in context")
		return
	}

	userUUID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid user ID")
		return
	}

	var input ChangePasswordRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: currentPassword and newPassword (min 8 chars) required")
		return
	}

	var user models.User
	result := h.DB.First(&user, "id = ?", userUUID)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			utils.RespondWithError(c, http.StatusNotFound, "User not found")
		} else {
			utils.RespondWithError(c, http.StatusInternalServerError, "DB error")
		}
		return
	}

	if !utils.CheckPasswordHash(input.CurrentPassword, user.Password) {
		utils.RespondWithError(c, http.StatusBadRequest, "Current password is incorrect")
		return
	}

	hashed, err := utils.HashPassword(input.NewPassword)
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to change password")
		return
	}

	if err := h.DB.Model(&user).Update("password", hashed).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to change password")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password changed successfully"})
}
