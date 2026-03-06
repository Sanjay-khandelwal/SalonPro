// controllers/service.go
package controllers

import (
	"errors"
	"net/http"
	"salonpro-backend/models"
	"salonpro-backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CreateServiceInput defines the expected JSON structure for creating a service
type CreateServiceInput struct {
	Name        string  `json:"name" binding:"required"`
	Description string  `json:"description"`
	Price       float64 `json:"price" binding:"required,min=0"`
	Duration    int     `json:"duration" binding:"min=0"` // in minutes
	Category    string  `json:"category"`
}

// UpdateServiceInput defines the expected JSON structure for updating a service
type UpdateServiceInput struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Price       *float64 `json:"price"`
	Duration    *int     `json:"duration"`
	Category    *string  `json:"category"`
	IsActive    *bool    `json:"isActive"`
}

// CreateService creates a new service for the salon
func (h *HandlerFunc) CreateService(c *gin.Context) {
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

	var input CreateServiceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}

	// Create new service
	service := models.Service{
		ID:          uuid.New(),
		SalonID:     salonUUID,
		Name:        input.Name,
		Description: input.Description,
		Price:       input.Price,
		Duration:    input.Duration,
		Category:    input.Category,
		IsActive:    true,
	}

	if err := h.DB.Create(&service).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to create service")
		return
	}

	c.JSON(http.StatusCreated, service)
}

// GetServices retrieves all services for the salon
func (h *HandlerFunc) GetServices(c *gin.Context) {
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

	var services []models.Service
	if err := h.DB.Where("salon_id = ?", salonUUID).Find(&services).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to retrieve services")
		return
	}

	c.JSON(http.StatusOK, services)
}

// GetService retrieves a specific service by ID
func (h *HandlerFunc) GetService(c *gin.Context) {
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

	serviceID := c.Param("id")
	serviceUUID, err := uuid.Parse(serviceID)
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid service ID format")
		return
	}

	var service models.Service
	if err := h.DB.Where("salon_id = ? AND id = ?", salonUUID, serviceUUID).
		First(&service).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.RespondWithError(c, http.StatusNotFound, "Service not found")
		} else {
			utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		}
		return
	}

	c.JSON(http.StatusOK, service)
}

// UpdateService updates an existing service
func (h *HandlerFunc) UpdateService(c *gin.Context) {
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

	serviceID := c.Param("id")
	serviceUUID, err := uuid.Parse(serviceID)
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid service ID format")
		return
	}

	var input UpdateServiceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}

	// Retrieve existing service
	var service models.Service
	if err := h.DB.Where("salon_id = ? AND id = ?", salonUUID, serviceUUID).
		First(&service).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.RespondWithError(c, http.StatusNotFound, "Service not found")
		} else {
			utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		}
		return
	}

	// Update fields if provided
	if input.Name != nil {
		service.Name = *input.Name
	}
	if input.Description != nil {
		service.Description = *input.Description
	}
	if input.Price != nil {
		service.Price = *input.Price
	}
	if input.Duration != nil {
		service.Duration = *input.Duration
	}
	if input.Category != nil {
		service.Category = *input.Category
	}
	if input.IsActive != nil {
		service.IsActive = *input.IsActive
	}

	if err := h.DB.Save(&service).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update service")
		return
	}

	c.JSON(http.StatusOK, service)
}

// DeleteService soft deletes a service
func (h *HandlerFunc) DeleteService(c *gin.Context) {
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

	serviceID := c.Param("id")
	serviceUUID, err := uuid.Parse(serviceID)
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid service ID format")
		return
	}

	result := h.DB.Where("salon_id = ? AND id = ?", salonUUID, serviceUUID).
		Delete(&models.Service{})

	if result.Error != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to delete service")
		return
	}

	if result.RowsAffected == 0 {
		utils.RespondWithError(c, http.StatusNotFound, "Service not found")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Service deleted successfully"})
}
