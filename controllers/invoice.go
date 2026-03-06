// controllers/invoice.go
package controllers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"salonpro-backend/models"
	"salonpro-backend/services"
	"salonpro-backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// InvoiceItemInput defines the structure for an invoice item
type InvoiceItemInput struct {
	ServiceID uuid.UUID `json:"serviceId" binding:"required"`
	Quantity  int       `json:"quantity" binding:"min=1"`
}

// CreateInvoiceInput defines the expected JSON structure for creating an invoice
type CreateInvoiceInput struct {
	CustomerID      uuid.UUID          `json:"customerId" binding:"required"`
	InvoiceDate     *time.Time         `json:"invoiceDate"`
	Items           []InvoiceItemInput `json:"items" binding:"required,min=1"`
	Discount        float64            `json:"discount" binding:"min=0"`
	Tax             float64            `json:"tax" binding:"min=0"`
	PaymentStatus   string             `json:"paymentStatus" binding:"oneof=paid unpaid partial"`
	PaidAmount      float64            `json:"paidAmount" binding:"min=0"`
	PaymentMethodID uuid.UUID          `json:"paymentMethodId" binding:"required"`
	Notes           string             `json:"notes"`
}

// UpdateInvoiceInput defines the expected JSON structure for updating an invoice
type UpdateInvoiceInput struct {
	CustomerID      *uuid.UUID          `json:"customerId"`
	InvoiceDate     *time.Time          `json:"invoiceDate"`
	Items           *[]InvoiceItemInput `json:"items"`
	Discount        *float64            `json:"discount"`
	Tax             *float64            `json:"tax"`
	PaymentStatus   *string             `json:"paymentStatus" binding:"omitempty,oneof=paid unpaid partial"`
	PaidAmount      *float64            `json:"paidAmount" binding:"omitempty,min=0"`
	PaymentMethodID *uuid.UUID          `json:"paymentMethodId"`
	Notes           *string             `json:"notes"`
}

// CreateInvoice creates a new invoice for the salon
func (h *HandlerFunc) CreateInvoice(c *gin.Context) {
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

	var input CreateInvoiceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}

	// Validate customer exists in the same salon
	var customer models.Customer
	if err := h.DB.Where("salon_id = ? AND id = ?", salonUUID, input.CustomerID).
		First(&customer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.RespondWithError(c, http.StatusBadRequest, "Customer not found")
		} else {
			utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		}
		return
	}

	// Validate payment method exists
	var pm models.PaymentMethod
	if err := h.DB.First(&pm, "id = ?", input.PaymentMethodID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.RespondWithError(c, http.StatusBadRequest, "Payment method not found")
		} else {
			utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		}
		return
	}

	// Validate and calculate invoice items
	var subtotal float64 = 0
	var invoiceItems []models.InvoiceItem

	for _, item := range input.Items {
		// Validate service exists and belongs to the same salon
		var service models.Service
		if err := h.DB.Where("salon_id = ? AND id = ?", salonUUID, item.ServiceID).
			First(&service).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				utils.RespondWithError(c, http.StatusBadRequest, "Service not found: "+item.ServiceID.String())
			} else {
				utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
			}
			return
		}

		// Calculate item total
		itemTotal := service.Price * float64(item.Quantity)
		subtotal += itemTotal

		invoiceItems = append(invoiceItems, models.InvoiceItem{
			ID:          uuid.New(),
			ServiceID:   service.ID,
			ServiceName: service.Name,
			Quantity:    item.Quantity,
			UnitPrice:   service.Price,
			TotalPrice:  itemTotal,
		})
	}

	// Calculate total
	total := subtotal - input.Discount + (subtotal * input.Tax / 100)

	// Set default invoice date to now if not provided
	invoiceDate := time.Now()
	if input.InvoiceDate != nil {
		invoiceDate = *input.InvoiceDate
	}

	// Create new invoice (include PaymentMethod association so PDF has the name)
	invoice := models.Invoice{
		ID:              uuid.New(),
		CreatedByUserID: uuid.Must(uuid.Parse(userID.(string))),
		SalonID:         salonUUID,
		PaymentMethodID: input.PaymentMethodID,
		PaymentMethod:   pm, // populate association so PDF/email can read pm.Name
		CustomerID:      input.CustomerID,
		InvoiceDate:     invoiceDate,
		Subtotal:        subtotal,
		Discount:        input.Discount,
		Tax:             input.Tax,
		Total:           total,
		PaymentStatus:   input.PaymentStatus,
		PaidAmount:      input.PaidAmount,
		Notes:           input.Notes,
		Items:           invoiceItems,
	}

	// Generate invoice number (you might want a better way)
	invoice.InvoiceNumber = "INV-" + time.Now().Format("20060102") + "-" + utils.GenerateRandomString(6)

	// Start transaction
	tx := h.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Save invoice
	if err := tx.Create(&invoice).Error; err != nil {
		tx.Rollback()
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to create invoice")
		return
	}

	// Update customer stats
	if err := tx.Model(&models.Customer{}).Where("id = ?", input.CustomerID).
		Updates(map[string]interface{}{
			"total_visits": gorm.Expr("total_visits + ?", 1),
			"total_spent":  gorm.Expr("total_spent + ?", total),
			"last_visit":   invoiceDate,
		}).Error; err != nil {
		tx.Rollback()
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update customer stats")
		return
	}

	tx.Commit()

	// Send invoice PDF to customer via email (async — does not block response)
	// Only sent if customer has an email and RESEND is configured.
	go func() {
		var salon models.Salon
		h.DB.First(&salon, "id = ?", salonUUID)

		pdfData := services.InvoicePDFData{
			Invoice:  invoice,
			Salon:    salon,
			Customer: customer,
		}
		pdfBytes, err := services.BuildInvoicePDF(pdfData)
		if err != nil {
			log.Printf("CreateInvoice: failed to build invoice PDF for email: %v", err)
			return
		}
		if err := h.Service.ResendService.SendInvoiceEmail(
			customer.Email,
			customer.Name,
			invoice.InvoiceNumber,
			salon.Name,
			pdfBytes,
		); err != nil {
			log.Printf("CreateInvoice: failed to send invoice email to %s: %v", customer.Email, err)
		}
	}()

	c.JSON(http.StatusCreated, invoice)
}

// GetInvoices retrieves all invoices for the salon with full associations
func (h *HandlerFunc) GetInvoices(c *gin.Context) {
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

	var invoices []models.Invoice
	if err := h.DB.
		Preload("Items").
		Preload("PaymentMethod").
		Preload("Customer").
		Preload("CreatedByUser").
		Where("salon_id = ?", salonUUID).
		Order("created_at DESC").
		Find(&invoices).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to retrieve invoices")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"invoices": formatInvoiceList(invoices),
		"total":    len(invoices),
	})
}

// GetInvoice retrieves a specific invoice by ID with full details
func (h *HandlerFunc) GetInvoice(c *gin.Context) {
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

	invoiceID := c.Param("id")
	invoiceUUID, err := uuid.Parse(invoiceID)
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid invoice ID format")
		return
	}

	var invoice models.Invoice
	if err := h.DB.
		Preload("Items").
		Preload("PaymentMethod").
		Preload("Customer").
		Preload("CreatedByUser").
		Where("salon_id = ? AND id = ?", salonUUID, invoiceUUID).
		First(&invoice).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.RespondWithError(c, http.StatusNotFound, "Invoice not found")
		} else {
			utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		}
		return
	}

	c.JSON(http.StatusOK, formatInvoice(invoice))
}

// formatInvoice returns a fully shaped invoice response with all nested details.
func formatInvoice(inv models.Invoice) gin.H {
	items := make([]gin.H, 0, len(inv.Items))
	for _, item := range inv.Items {
		items = append(items, gin.H{
			"id":          item.ID,
			"serviceId":   item.ServiceID,
			"serviceName": item.ServiceName,
			"quantity":    item.Quantity,
			"unitPrice":   item.UnitPrice,
			"totalPrice":  item.TotalPrice,
		})
	}

	return gin.H{
		"id":            inv.ID,
		"invoiceNumber": inv.InvoiceNumber,
		"invoiceDate":   inv.InvoiceDate,
		"createdAt":     inv.CreatedAt,
		"updatedAt":     inv.UpdatedAt,

		"customer": gin.H{
			"id":    inv.Customer.ID,
			"name":  inv.Customer.Name,
			"phone": inv.Customer.Phone,
			"email": inv.Customer.Email,
		},

		"paymentMethod": gin.H{
			"id":   inv.PaymentMethod.ID,
			"name": inv.PaymentMethod.Name,
			"type": inv.PaymentMethod.Type,
		},

		"paymentStatus": inv.PaymentStatus,
		"paidAmount":    inv.PaidAmount,
		"balanceDue":    inv.Total - inv.PaidAmount,

		"subtotal": inv.Subtotal,
		"discount": inv.Discount,
		"tax":      inv.Tax,
		"total":    inv.Total,

		"notes": inv.Notes,
		"items": items,

		"createdBy": gin.H{
			"id":   inv.CreatedByUser.ID,
			"name": inv.CreatedByUser.Name,
		},
	}
}

// formatInvoiceList shapes a list of invoices for the list endpoint.
func formatInvoiceList(invoices []models.Invoice) []gin.H {
	result := make([]gin.H, 0, len(invoices))
	for _, inv := range invoices {
		result = append(result, formatInvoice(inv))
	}
	return result
}

// UpdateInvoice updates an existing invoice
func (h *HandlerFunc) UpdateInvoice(c *gin.Context) {
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

	invoiceID := c.Param("id")
	invoiceUUID, err := uuid.Parse(invoiceID)
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid invoice ID format")
		return
	}

	var input UpdateInvoiceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid input: "+err.Error())
		return
	}

	// Start transaction
	tx := h.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Retrieve existing invoice
	var invoice models.Invoice
	if err := tx.Preload("Items").Preload("PaymentMethod").
		Where("salon_id = ? AND id = ?", salonUUID, invoiceUUID).
		First(&invoice).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.RespondWithError(c, http.StatusNotFound, "Invoice not found")
		} else {
			utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		}
		return
	}

	// Update fields if provided
	if input.CustomerID != nil {
		// Validate customer exists in the same salon
		var customer models.Customer
		if err := tx.Where("salon_id = ? AND id = ?", salonUUID, *input.CustomerID).
			First(&customer).Error; err != nil {
			tx.Rollback()
			if errors.Is(err, gorm.ErrRecordNotFound) {
				utils.RespondWithError(c, http.StatusBadRequest, "Customer not found")
			} else {
				utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
			}
			return
		}
		invoice.CustomerID = *input.CustomerID
	}

	if input.InvoiceDate != nil {
		invoice.InvoiceDate = *input.InvoiceDate
	}

	// If items are being updated, recalculate the invoice
	if input.Items != nil {
		var subtotal float64 = 0
		var newInvoiceItems []models.InvoiceItem

		// Delete existing items
		if err := tx.Where("invoice_id = ?", invoice.ID).Delete(&models.InvoiceItem{}).Error; err != nil {
			tx.Rollback()
			utils.RespondWithError(c, http.StatusInternalServerError, "Failed to clear existing items")
			return
		}

		// Create new items
		for _, item := range *input.Items {
			// Validate service exists and belongs to the same salon
			var service models.Service
			if err := tx.Where("salon_id = ? AND id = ?", salonUUID, item.ServiceID).
				First(&service).Error; err != nil {
				tx.Rollback()
				if errors.Is(err, gorm.ErrRecordNotFound) {
					utils.RespondWithError(c, http.StatusBadRequest, "Service not found: "+item.ServiceID.String())
				} else {
					utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
				}
				return
			}

			// Calculate item total
			itemTotal := service.Price * float64(item.Quantity)
			subtotal += itemTotal

			newInvoiceItems = append(newInvoiceItems, models.InvoiceItem{
				InvoiceID:   invoice.ID,
				ServiceID:   service.ID,
				ServiceName: service.Name,
				Quantity:    item.Quantity,
				UnitPrice:   service.Price,
				TotalPrice:  itemTotal,
			})
		}

		for i := range newInvoiceItems {
			if err := tx.Create(&newInvoiceItems[i]).Error; err != nil {
				tx.Rollback()
				utils.RespondWithError(c, http.StatusInternalServerError, "Failed to create invoice items")
				return
			}
		}
		invoice.Items = newInvoiceItems
		invoice.Subtotal = subtotal
	}

	if input.Discount != nil {
		invoice.Discount = *input.Discount
	}

	if input.Tax != nil {
		invoice.Tax = *input.Tax
	}

	// Recalculate total if needed
	if input.Items != nil || input.Discount != nil || input.Tax != nil {
		invoice.Total = invoice.Subtotal - invoice.Discount + (invoice.Subtotal * invoice.Tax / 100)
	}

	if input.PaymentStatus != nil {
		invoice.PaymentStatus = *input.PaymentStatus
	}

	if input.PaidAmount != nil {
		invoice.PaidAmount = *input.PaidAmount
	}

	if input.PaymentMethodID != nil {
		var pm models.PaymentMethod
		if err := tx.First(&pm, "id = ?", *input.PaymentMethodID).Error; err != nil {
			tx.Rollback()
			if errors.Is(err, gorm.ErrRecordNotFound) {
				utils.RespondWithError(c, http.StatusBadRequest, "Payment method not found")
			} else {
				utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
			}
			return
		}
		invoice.PaymentMethodID = *input.PaymentMethodID
		invoice.PaymentMethod = pm // keep association in sync for PDF/email
	}

	if input.Notes != nil {
		invoice.Notes = *input.Notes
	}

	// Save updated invoice
	if err := tx.Save(&invoice).Error; err != nil {
		tx.Rollback()
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update invoice")
		return
	}

	tx.Commit()

	// Reload full invoice with all associations for response + email
	var updatedInvoice models.Invoice
	h.DB.
		Preload("Items").
		Preload("PaymentMethod").
		Preload("Customer").
		Preload("CreatedByUser").
		Where("salon_id = ? AND id = ?", salonUUID, invoiceUUID).
		First(&updatedInvoice)

	// Send updated invoice PDF to customer via email (async — does not block response)
	go func() {
		var salon models.Salon
		h.DB.First(&salon, "id = ?", salonUUID)

		pdfData := services.InvoicePDFData{
			Invoice:  updatedInvoice,
			Salon:    salon,
			Customer: updatedInvoice.Customer,
		}
		pdfBytes, err := services.BuildInvoicePDF(pdfData)
		if err != nil {
			log.Printf("UpdateInvoice: failed to build invoice PDF for email: %v", err)
			return
		}
		if err := h.Service.ResendService.SendInvoiceEmail(
			updatedInvoice.Customer.Email,
			updatedInvoice.Customer.Name,
			updatedInvoice.InvoiceNumber,
			salon.Name,
			pdfBytes,
		); err != nil {
			log.Printf("UpdateInvoice: failed to send invoice email to %s: %v", updatedInvoice.Customer.Email, err)
		}
	}()

	c.JSON(http.StatusOK, formatInvoice(updatedInvoice))
}

// DeleteInvoice soft deletes an invoice
func (h *HandlerFunc) DeleteInvoice(c *gin.Context) {
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

	invoiceID := c.Param("id")
	invoiceUUID, err := uuid.Parse(invoiceID)
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid invoice ID format")
		return
	}

	// Start transaction
	tx := h.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Retrieve invoice to get customer and total
	var invoice models.Invoice
	if err := tx.Where("salon_id = ? AND id = ?", salonUUID, invoiceUUID).
		First(&invoice).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.RespondWithError(c, http.StatusNotFound, "Invoice not found")
		} else {
			utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		}
		return
	}

	// Delete invoice items
	if err := tx.Where("invoice_id = ?", invoice.ID).Delete(&models.InvoiceItem{}).Error; err != nil {
		tx.Rollback()
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to delete invoice items")
		return
	}

	// Delete invoice
	if err := tx.Delete(&invoice).Error; err != nil {
		tx.Rollback()
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to delete invoice")
		return
	}

	// Update customer stats (decrement)
	if err := tx.Model(&models.Customer{}).Where("id = ?", invoice.CustomerID).
		Updates(map[string]interface{}{
			"total_visits": gorm.Expr("total_visits - ?", 1),
			"total_spent":  gorm.Expr("total_spent - ?", invoice.Total),
		}).Error; err != nil {
		tx.Rollback()
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to update customer stats")
		return
	}

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{"message": "Invoice deleted successfully"})
}

// GetInvoicePDF streams a PDF for the given invoice ID via query param ?id=<invoiceId>.
// No file is stored on disk — the PDF is generated in memory (via services.BuildInvoicePDF) and sent directly.
// GET /api/invoices/pdf?id=<uuid>
func (h *HandlerFunc) GetInvoicePDF(c *gin.Context) {
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

	invoiceIDStr := c.Query("id")
	if invoiceIDStr == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "Query param 'id' is required")
		return
	}
	invoiceUUID, err := uuid.Parse(invoiceIDStr)
	if err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "Invalid invoice ID format")
		return
	}

	var invoice models.Invoice
	if err := h.DB.Preload("Items").Preload("PaymentMethod").
		Where("salon_id = ? AND id = ?", salonUUID, invoiceUUID).
		First(&invoice).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.RespondWithError(c, http.StatusNotFound, "Invoice not found")
		} else {
			utils.RespondWithError(c, http.StatusInternalServerError, "Database error")
		}
		return
	}

	var salon models.Salon
	h.DB.First(&salon, "id = ?", salonUUID)

	var customer models.Customer
	h.DB.First(&customer, "id = ?", invoice.CustomerID)

	// Reuse shared PDF builder (no disk I/O)
	pdfBytes, err := services.BuildInvoicePDF(services.InvoicePDFData{
		Invoice:  invoice,
		Salon:    salon,
		Customer: customer,
	})
	if err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to generate PDF")
		return
	}

	filename := fmt.Sprintf("invoice-%s.pdf", invoice.InvoiceNumber)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Length", fmt.Sprintf("%d", len(pdfBytes)))
	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}

// GetPaymentMethods returns all payment methods (for invoice create/edit dropdowns).
func (h *HandlerFunc) GetPaymentMethods(c *gin.Context) {
	var methods []models.PaymentMethod
	if err := h.DB.Order("name").Find(&methods).Error; err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, "Failed to retrieve payment methods")
		return
	}
	c.JSON(http.StatusOK, methods)
}
