package controllers

import (
	"fmt"
	"net/http"
	"salonpro-backend/models"
	"salonpro-backend/utils"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type DashboardOverview struct {
	TotalCustomers    int                `json:"totalCustomers"`
	MonthlyRevenue    float64            `json:"monthlyRevenue"`
	TotalInvoices     int                `json:"totalInvoices"`
	UpcomingBirthdays []UpcomingEvent    `json:"upcomingBirthdays"`
	RecentCustomers   []RecentCustomer   `json:"recentCustomers"`
	UpcomingReminders []UpcomingReminder `json:"upcomingReminders"`
}

type UpcomingEvent struct {
	Name string `json:"name"`
	Date string `json:"date"` // e.g. "Tomorrow", "3 days", etc.
}

type RecentCustomer struct {
	Name      string `json:"name"`
	Service   string `json:"service"`
	VisitDate string `json:"visitDate"` // e.g. "Today", "Yesterday"
}

type UpcomingReminder struct {
	Name string `json:"name"`
	Type string `json:"type"` // "Birthday" or "Anniversary"
	Date string `json:"date"` // e.g. "Tomorrow", "3 days"
}

func (h *HandlerFunc) GetDashboardOverview(c *gin.Context) {
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

	// Total Customers
	var totalCustomers int64
	h.DB.Model(&models.Customer{}).Where("salon_id = ?", salonUUID).Count(&totalCustomers)

	// This Month's Revenue
	now := time.Now()
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	var monthlyRevenue float64
	h.DB.Model(&models.Invoice{}).
		Where("salon_id = ? AND invoice_date >= ?", salonUUID, firstOfMonth).
		Select("COALESCE(SUM(total), 0)").Scan(&monthlyRevenue)

	// Total Invoices
	var totalInvoices int64
	h.DB.Model(&models.Invoice{}).Where("salon_id = ?", salonUUID).Count(&totalInvoices)

	// Upcoming Birthdays (till end of year, ignore year part)
	var birthdayCount int64
	h.DB.Raw(`
        SELECT COUNT(*) FROM customers
        WHERE salon_id = ?
        AND (
            (EXTRACT(MONTH FROM birthday) > ?) OR
            (EXTRACT(MONTH FROM birthday) = ? AND EXTRACT(DAY FROM birthday) >= ?)
        )
    `, salonUUID, int(now.Month()), int(now.Month()), now.Day()).Scan(&birthdayCount)

	// List of upcoming birthdays (optional, for display)
	var upcomingBirthdays []UpcomingEvent
	h.DB.Raw(`
        SELECT name, TO_CHAR(birthday, 'MM-DD') as date FROM customers
        WHERE salon_id = ?
        AND (
            (EXTRACT(MONTH FROM birthday) > ?) OR
            (EXTRACT(MONTH FROM birthday) = ? AND EXTRACT(DAY FROM birthday) >= ?)
        )
        ORDER BY EXTRACT(MONTH FROM birthday), EXTRACT(DAY FROM birthday)
        LIMIT 7
    `, salonUUID, int(now.Month()), int(now.Month()), now.Day()).Scan(&upcomingBirthdays)

	// Recent Customers (last 3 visits)
	var recentCustomers []RecentCustomer
	rows, err := h.DB.Raw(`
    SELECT c.name, i.invoice_date, i.id
    FROM invoices i
    JOIN customers c ON c.id = i.customer_id
    WHERE i.salon_id = ?
    ORDER BY i.invoice_date DESC
`, salonUUID).Rows()
	if err == nil {
		defer rows.Close()
		customerMap := make(map[string]bool)
		count := 0
		for rows.Next() {
			var name string
			var invoiceDate time.Time
			var invoiceID uuid.UUID
			rows.Scan(&name, &invoiceDate, &invoiceID)
			if customerMap[name] {
				continue
			}
			// Get all services for this invoice
			var services []string
			h.DB.Raw(`
            SELECT service_name FROM invoice_items WHERE invoice_id = ?
        `, invoiceID).Scan(&services)
			// Calculate "Today", "Yesterday", "X days ago"
			daysAgo := int(time.Since(invoiceDate).Hours() / 24)
			var visitDate string
			switch daysAgo {
			case 0:
				visitDate = "Today"
			case 1:
				visitDate = "Yesterday"
			default:
				visitDate = fmt.Sprintf("%d days ago", daysAgo)
			}
			recentCustomers = append(recentCustomers, RecentCustomer{
				Name:      name,
				Service:   strings.Join(services, ", "),
				VisitDate: visitDate,
			})
			customerMap[name] = true
			count++
			if count >= 3 {
				break
			}
		}
	}

	// Upcoming Reminders (next 7 days, birthdays/anniversaries)
	var upcomingReminders []UpcomingReminder
	type reminderRow struct {
		Name string
		Type string
		Date time.Time
	}
	var reminders []reminderRow
	h.DB.Raw(`
    SELECT name, 'Birthday' as type, birthday as date
    FROM customers
    WHERE salon_id = ? AND birthday IS NOT NULL
    UNION ALL
    SELECT name, 'Anniversary' as type, anniversary as date
    FROM customers
    WHERE salon_id = ? AND anniversary IS NOT NULL
`, salonUUID, salonUUID).Scan(&reminders)

	today := time.Now()
	for _, r := range reminders {
		// Set year to this year for comparison
		eventDate := time.Date(today.Year(), r.Date.Month(), r.Date.Day(), 0, 0, 0, 0, today.Location())
		daysUntil := int(eventDate.Sub(today).Hours() / 24)
		if daysUntil < 0 {
			// If already passed, skip (or handle next year if you want)
			continue
		}
		if daysUntil > 6 {
			continue
		}
		var label string
		switch daysUntil {
		case 0:
			label = "Today"
		case 1:
			label = "Tomorrow"
		default:
			label = fmt.Sprintf("%d days", daysUntil)
		}
		upcomingReminders = append(upcomingReminders, UpcomingReminder{
			Name: r.Name,
			Type: r.Type,
			Date: label,
		})
		if len(upcomingReminders) >= 7 {
			break
		}
	}

	// Compose response
	response := gin.H{
		"totalCustomers": totalCustomers,
		"monthlyRevenue": monthlyRevenue,
		"totalInvoices":  totalInvoices,
		"upcomingBirthdays": gin.H{
			"count": birthdayCount,
			"list":  upcomingBirthdays,
		},
		"recentCustomers":   recentCustomers,
		"upcomingReminders": upcomingReminders,
	}

	c.JSON(http.StatusOK, response)
}
