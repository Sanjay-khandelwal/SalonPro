package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"salonpro-backend/models"
	"salonpro-backend/utils"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CreateCustomerInput defines the expected JSON structure for creating a customer
type CreateCustomerInput struct {
	Name        string     `json:"name" binding:"required"`
	Phone       string     `json:"phone" binding:"required"`
	Email       *string    `json:"email"` // Pointer to allow null
	Birthday    *time.Time `json:"birthday"`
	Anniversary *time.Time `json:"anniversary"`
	Notes       string     `json:"notes"`
}

// UpdateCustomerInput defines the expected JSON structure for updating a customer
type UpdateCustomerInput struct {
	Name        *string    `json:"name"`
	Phone       *string    `json:"phone"`
	Email       *string    `json:"email"`
	Birthday    *time.Time `json:"birthday"`
	Anniversary *time.Time `json:"anniversary"`
	Notes       *string    `json:"notes"`
	IsActive    *bool      `json:"isActive"`
}

// CreateCustomer creates a new customer for the salon
func (h *HandlerFunc) CreateCustomer(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found in context")
		return
	}
	userID, exists := c.Get("userId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "User ID not found in context")
		return
	}

	salonUUID, err := uuid.Parse(salonID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Invalid salon ID format")
		return
	}

	var input CreateCustomerInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}

	// Validate phone format
	if !utils.ValidatePhone(input.Phone) {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid phone number format")
		return
	}

	// Check if phone already exists for this salon
	var existingCustomer models.Customer
	if err := h.DB.Where("salon_id = ? AND email = ?", salonUUID, input.Email).
		First(&existingCustomer).Error; err == nil {
		utils.RespondWithError(c, http.StatusConflict, "Customer with this Email already exists")
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		fmt.Println("err", err.Error())
		utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		return
	}

	// Create new customer
	customer := models.Customer{
		ID:              uuid.New(),
		SalonID:         salonUUID,
		CreatedByUserID: uuid.Must(uuid.Parse(userID.(string))),
		Name:            input.Name,
		Phone:           input.Phone,
		Birthday:        input.Birthday,
		Anniversary:     input.Anniversary,
		Notes:           input.Notes,
		IsActive:        true,
	}

	if input.Email != nil {
		customer.Email = *input.Email
	}

	if err := h.DB.Create(&customer).Error; err != nil {
		fmt.Println(err.Error())
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to create customer "+err.Error())
		return
	}

	// send welcome email asynchronously — never block the HTTP response
	if input.Email != nil {
		emailCopy := *input.Email
		nameCopy := input.Name
		go func() {
			if err := h.Service.ResendService.SendCustomerWelcomeEmail(emailCopy, nameCopy); err != nil {
				fmt.Printf("CreateCustomer: failed to send welcome email to %s: %v\n", emailCopy, err)
			}
		}()
	}

	// send welcome SMS asynchronously — never block the HTTP response
	phoneCopy := input.Phone
	nameCopyForSMS := input.Name
	go func() {
		if err := h.Service.TwilioService.SendCustomerWelcomeSMS(phoneCopy, nameCopyForSMS); err != nil {
			fmt.Printf("CreateCustomer: failed to send welcome SMS to %s: %v\n", phoneCopy, err)
		}
	}()

	c.JSON(http.StatusCreated, customer)
}

// GetCustomers retrieves all customers for the salon
func (h *HandlerFunc) GetCustomers(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found in context")
		return
	}

	salonUUID, err := uuid.Parse(salonID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Invalid salon ID format")
		return
	}

	var customers []models.Customer
	if err := h.DB.Where("salon_id = ?", salonUUID).Find(&customers).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to retrieve customers")
		return
	}

	c.JSON(http.StatusOK, customers)
}

// GetCustomer retrieves a specific customer by ID
func (h *HandlerFunc) GetCustomer(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found in context")
		return
	}

	salonUUID, err := uuid.Parse(salonID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Invalid salon ID format")
		return
	}

	customerID := c.Param("id")
	customerUUID, err := uuid.Parse(customerID)
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid customer ID format")
		return
	}

	var customer models.Customer
	if err := h.DB.Where("salon_id = ? AND id = ?", salonUUID, customerUUID).
		First(&customer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.RespondWithError(c, http.StatusNotFound, "Customer not found")
		} else {
			utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		}
		return
	}

	c.JSON(http.StatusOK, customer)
}

// UpdateCustomer updates an existing customer
func (h *HandlerFunc) UpdateCustomer(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found in context")
		return
	}

	salonUUID, err := uuid.Parse(salonID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Invalid salon ID format")
		return
	}

	customerID := c.Param("id")
	customerUUID, err := uuid.Parse(customerID)
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid customer ID format")
		return
	}

	var input UpdateCustomerInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}

	// Retrieve existing customer
	var customer models.Customer
	if err := h.DB.Where("salon_id = ? AND id = ?", salonUUID, customerUUID).
		First(&customer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.RespondWithError(c, http.StatusNotFound, "Customer not found")
		} else {
			utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		}
		return
	}

	// Update fields if provided
	if input.Name != nil {
		customer.Name = *input.Name
	}
	if input.Phone != nil {
		// Validate phone format
		if !utils.ValidatePhone(*input.Phone) {
			utils.RespondWithError(c, http.StatusBadRequest, "Invalid phone number format")
			return
		}

		// Check if phone is being changed to another existing customer
		if customer.Phone != *input.Phone {
			var existingCustomer models.Customer
			if err := h.DB.Where("salon_id = ? AND phone = ?", salonUUID, *input.Phone).
				First(&existingCustomer).Error; err == nil {
				utils.RespondWithError(c, http.StatusConflict, "Another customer with this phone number already exists")
				return
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
				return
			}
		}
		customer.Phone = *input.Phone
	}
	if input.Email != nil {
		customer.Email = *input.Email
	}
	if input.Birthday != nil {
		customer.Birthday = input.Birthday
	}
	if input.Anniversary != nil {
		customer.Anniversary = input.Anniversary
	}
	if input.Notes != nil {
		customer.Notes = *input.Notes
	}
	if input.IsActive != nil {
		customer.IsActive = *input.IsActive
	}

	if err := h.DB.Save(&customer).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update customer")
		return
	}

	c.JSON(http.StatusOK, customer)
}

// DeleteCustomer soft deletes a customer
func (h *HandlerFunc) DeleteCustomer(c *gin.Context) {
	salonID, exists := c.Get("salonId")
	if !exists {
		utils.RespondWithError(c, http.StatusUnauthorized, "Salon ID not found in context")
		return
	}

	salonUUID, err := uuid.Parse(salonID.(string))
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Invalid salon ID format")
		return
	}

	customerID := c.Param("id")
	customerUUID, err := uuid.Parse(customerID)
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid customer ID format")
		return
	}

	result := h.DB.Where("salon_id = ? AND id = ?", salonUUID, customerUUID).
		Delete(&models.Customer{})

	if result.Error != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to delete customer")
		return
	}

	if result.RowsAffected == 0 {
		utils.RespondWithError(c, http.StatusNotFound, "Customer not found")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Customer deleted successfully"})
}
