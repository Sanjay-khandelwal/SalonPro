// controllers/report.go
package controllers

import (
	"fmt"
	"net/http"
	"salonpro-backend/utils"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ReportController handles all reporting functions
type ReportController struct {
	DB *gorm.DB
}

func NewReportAnalysis(db *gorm.DB) *ReportController {
	return &ReportController{
		DB: db,
	}
}

// AnalyticsSummary represents the Analytics data
type AnalyticsSummary struct {
	CurrentMonthRevenue    float64                `json:"currentMonthRevenue"`
	MonthGrowth            float64                `json:"monthGrowth"`
	CurrentQuarterRevenue  float64                `json:"currentQuarterRevenue"`
	QuarterGrowth          float64                `json:"quarterGrowth"`
	CurrentYearRevenue     float64                `json:"currentYearRevenue"`
	YearGrowth             float64                `json:"yearGrowth"`
	TopServices            []ServiceSummary       `json:"topServices"`
	TopCustomers           []CustomerSummary      `json:"topCustomers"`
	QuickStats             QuickStatistics        `json:"quickStats"`
	TopEmployees           []EmployeeSummary      `json:"topEmployees"`
	EmployeeServiceSummary []EmployeeServiceStats `json:"employeeServiceSummary"`
}

type ServiceSummary struct {
	Name    string  `json:"name"`
	Count   int     `json:"count"`
	Revenue float64 `json:"revenue"`
}

type CustomerSummary struct {
	Name   string  `json:"name"`
	Visits int     `json:"visits"`
	Spent  float64 `json:"spent"`
}

type QuickStatistics struct {
	TotalCustomers   int     `json:"totalCustomers"`
	TotalInvoices    int     `json:"totalInvoices"`
	AvgMonthlyVisits float64 `json:"avgMonthlyVisits"`
	AvgOrderValue    float64 `json:"avgOrderValue"`
}

type EmployeeSummary struct {
	Name            string  `json:"name"`
	Revenue         float64 `json:"revenue"`
	ServicesHandled int     `json:"servicesHandled"`
}

type EmployeeServiceStats struct {
	EmployeeName string  `json:"employeeName"`
	ServiceName  string  `json:"serviceName"`
	Count        int     `json:"count"`
	Revenue      float64 `json:"revenue"`
}

// RevenueData holds consolidated revenue information
type RevenueData struct {
	CurrentMonth   float64
	LastMonth      float64
	CurrentQuarter float64
	LastQuarter    float64
	CurrentYear    float64
	LastYear       float64
}

// GetReportAnalytics returns the complete dashboard summary with optimizations
func (rc *ReportController) GetReportAnalytics(c *gin.Context) {
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

	now := time.Now()
	currentYear, currentMonth, _ := now.Date()
	currentLocation := now.Location()

	// Calculate all date ranges once
	firstOfMonth := time.Date(currentYear, currentMonth, 1, 0, 0, 0, 0, currentLocation)
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)

	// Use goroutines to fetch data concurrently
	var wg sync.WaitGroup
	var mu sync.Mutex

	var revenueData RevenueData
	var topServices []ServiceSummary
	var topCustomers []CustomerSummary
	var quickStats QuickStatistics
	var topEmployees []EmployeeSummary
	var employeeServiceStats []EmployeeServiceStats
	var errors []error

	addError := func(err error) {
		mu.Lock()
		errors = append(errors, err)
		mu.Unlock()
	}

	// Fetch revenue data (consolidated query)
	wg.Add(1)
	go func() {
		defer wg.Done()
		data, err := rc.getConsolidatedRevenueData(salonUUID, now)
		if err != nil {
			addError(fmt.Errorf("failed to get revenue data: %w", err))
			return
		}
		revenueData = data
	}()

	// Fetch top services
	wg.Add(1)
	go func() {
		defer wg.Done()
		services, err := rc.getTopServices(salonUUID, firstOfMonth, lastOfMonth, 4)
		if err != nil {
			addError(fmt.Errorf("failed to get top services: %w", err))
			return
		}
		topServices = services
	}()

	// Fetch top customers
	wg.Add(1)
	go func() {
		defer wg.Done()
		customers, err := rc.getTopCustomers(salonUUID, firstOfMonth, lastOfMonth, 4)
		if err != nil {
			addError(fmt.Errorf("failed to get top customers: %w", err))
			return
		}
		topCustomers = customers
	}()

	// Fetch quick statistics
	wg.Add(1)
	go func() {
		defer wg.Done()
		stats, err := rc.getQuickStatistics(salonUUID)
		if err != nil {
			addError(fmt.Errorf("failed to get quick statistics: %w", err))
			return
		}
		quickStats = stats
	}()

	// Fetch top employees
	wg.Add(1)
	go func() {
		defer wg.Done()
		employees, err := rc.getTopEmployees(salonUUID, firstOfMonth, lastOfMonth, 4)
		if err != nil {
			addError(fmt.Errorf("failed to get top employees: %w", err))
			return
		}
		topEmployees = employees
	}()

	// Fetch employee service distribution
	wg.Add(1)
	go func() {
		defer wg.Done()
		stats, err := rc.getEmployeeServiceDistribution(salonUUID, firstOfMonth, lastOfMonth)
		if err != nil {
			addError(fmt.Errorf("failed to get employee service distribution: %w", err))
			return
		}
		employeeServiceStats = stats
	}()

	// Wait for all goroutines to complete
	wg.Wait()

	// Check for errors
	if len(errors) > 0 {
		utils.RespondWithError(c, http.StatusInternalServerError, fmt.Sprintf("Failed to fetch data: %v", errors[0]))
		return
	}

	// Calculate growth percentages
	monthGrowth := rc.calculateGrowthPercentage(revenueData.CurrentMonth, revenueData.LastMonth)
	quarterGrowth := rc.calculateGrowthPercentage(revenueData.CurrentQuarter, revenueData.LastQuarter)
	yearGrowth := rc.calculateGrowthPercentage(revenueData.CurrentYear, revenueData.LastYear)

	// Create response
	summary := AnalyticsSummary{
		CurrentMonthRevenue:    revenueData.CurrentMonth,
		MonthGrowth:            monthGrowth,
		CurrentQuarterRevenue:  revenueData.CurrentQuarter,
		QuarterGrowth:          quarterGrowth,
		CurrentYearRevenue:     revenueData.CurrentYear,
		YearGrowth:             yearGrowth,
		TopServices:            topServices,
		TopCustomers:           topCustomers,
		QuickStats:             quickStats,
		TopEmployees:           topEmployees,
		EmployeeServiceSummary: employeeServiceStats,
	}

	c.JSON(http.StatusOK, summary)
}

// getConsolidatedRevenueData fetches all revenue data in a single optimized query
func (rc *ReportController) getConsolidatedRevenueData(salonID uuid.UUID, now time.Time) (RevenueData, error) {
	var data RevenueData
	currentYear, currentMonth, _ := now.Date()
	currentLocation := now.Location()

	// Calculate all date ranges
	firstOfMonth := time.Date(currentYear, currentMonth, 1, 0, 0, 0, 0, currentLocation)
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)

	firstOfLastMonth := firstOfMonth.AddDate(0, -1, 0)
	lastOfLastMonth := firstOfMonth.AddDate(0, 0, -1)

	quarterStart := rc.getQuarterStart(now)
	quarterEnd := rc.getQuarterEnd(now)
	lastQuarterStart := quarterStart.AddDate(0, -3, 0)
	lastQuarterEnd := quarterEnd.AddDate(0, -3, 0)

	yearStart := time.Date(currentYear, 1, 1, 0, 0, 0, 0, currentLocation)
	yearEnd := time.Date(currentYear, 12, 31, 23, 59, 59, 0, currentLocation)
	lastYearStart := time.Date(currentYear-1, 1, 1, 0, 0, 0, 0, currentLocation)
	lastYearEnd := time.Date(currentYear-1, 12, 31, 23, 59, 59, 0, currentLocation)

	// Single query to get all revenue data
	query := `
		SELECT 
			COALESCE(SUM(CASE WHEN invoice_date BETWEEN ? AND ? THEN total ELSE 0 END), 0) as current_month,
			COALESCE(SUM(CASE WHEN invoice_date BETWEEN ? AND ? THEN total ELSE 0 END), 0) as last_month,
			COALESCE(SUM(CASE WHEN invoice_date BETWEEN ? AND ? THEN total ELSE 0 END), 0) as current_quarter,
			COALESCE(SUM(CASE WHEN invoice_date BETWEEN ? AND ? THEN total ELSE 0 END), 0) as last_quarter,
			COALESCE(SUM(CASE WHEN invoice_date BETWEEN ? AND ? THEN total ELSE 0 END), 0) as current_year,
			COALESCE(SUM(CASE WHEN invoice_date BETWEEN ? AND ? THEN total ELSE 0 END), 0) as last_year
		FROM invoices 
		WHERE salon_id = ?
`

	var result struct {
		CurrentMonth   float64 `db:"current_month"`
		LastMonth      float64 `db:"last_month"`
		CurrentQuarter float64 `db:"current_quarter"`
		LastQuarter    float64 `db:"last_quarter"`
		CurrentYear    float64 `db:"current_year"`
		LastYear       float64 `db:"last_year"`
	}

	err := rc.DB.Raw(query,
		firstOfMonth, lastOfMonth, // current month
		firstOfLastMonth, lastOfLastMonth, // last month
		quarterStart, quarterEnd, // current quarter
		lastQuarterStart, lastQuarterEnd, // last quarter
		yearStart, yearEnd, // current year
		lastYearStart, lastYearEnd, // last year
		salonID, // salon_id
	).Scan(&result).Error

	if err != nil {
		return data, err
	}

	data.CurrentMonth = result.CurrentMonth
	data.LastMonth = result.LastMonth
	data.CurrentQuarter = result.CurrentQuarter
	data.LastQuarter = result.LastQuarter
	data.CurrentYear = result.CurrentYear
	data.LastYear = result.LastYear

	return data, nil
}

// getQuickStatistics optimized with a single query
func (rc *ReportController) getQuickStatistics(salonID uuid.UUID) (QuickStatistics, error) {
	var stats QuickStatistics

	// Single query to get all statistics
	query := `
		SELECT 
			(SELECT COUNT(*) FROM customers WHERE salon_id = ?) as total_customers,
			(SELECT COUNT(*) FROM invoices WHERE salon_id = ?) as total_invoices,
			(SELECT COALESCE(SUM(total), 0) FROM invoices WHERE salon_id = ?) as total_revenue,
			(SELECT COALESCE(AVG(visits), 0) FROM (
				SELECT COUNT(*) as visits
				FROM invoices
				WHERE salon_id = ?
				GROUP BY DATE_TRUNC('month', invoice_date)
			) monthly_visits) as avg_monthly_visits
	`

	var result struct {
		TotalCustomers   int     `db:"total_customers"`
		TotalInvoices    int     `db:"total_invoices"`
		TotalRevenue     float64 `db:"total_revenue"`
		AvgMonthlyVisits float64 `db:"avg_monthly_visits"`
	}

	err := rc.DB.Raw(query, salonID, salonID, salonID, salonID).Scan(&result).Error
	if err != nil {
		return stats, err
	}

	stats.TotalCustomers = result.TotalCustomers
	stats.TotalInvoices = result.TotalInvoices
	stats.AvgMonthlyVisits = result.AvgMonthlyVisits

	// Calculate average order value
	if result.TotalInvoices > 0 {
		stats.AvgOrderValue = result.TotalRevenue / float64(result.TotalInvoices)
	}

	return stats, nil
}

// Optimized helper functions with better indexing hints

func (rc *ReportController) getTopServices(salonID uuid.UUID, start, end time.Time, limit int) ([]ServiceSummary, error) {
	var services []ServiceSummary

	// Optimized query with proper joins and indexing
	query := `
		SELECT s.name, 
			   SUM(ii.quantity) as count, 
			   SUM(ii.total_price) as revenue
		FROM invoice_items ii
		INNER JOIN invoices i ON i.id = ii.invoice_id 
		INNER JOIN services s ON s.id = ii.service_id
		WHERE i.salon_id = ? 
		  AND i.invoice_date BETWEEN ? AND ?
		GROUP BY s.id, s.name
		ORDER BY revenue DESC
		LIMIT ?
	`

	err := rc.DB.Raw(query, salonID, start, end, limit).Scan(&services).Error
	return services, err
}

func (rc *ReportController) getTopCustomers(salonID uuid.UUID, start, end time.Time, limit int) ([]CustomerSummary, error) {
	var customers []CustomerSummary

	query := `
		SELECT c.name, 
			   COUNT(i.id) as visits, 
			   SUM(i.total) as spent
		FROM invoices i
		INNER JOIN customers c ON c.id = i.customer_id
		WHERE i.salon_id = ? 
		  AND i.invoice_date BETWEEN ? AND ?
		GROUP BY c.id, c.name
		ORDER BY spent DESC
		LIMIT ?
	`

	err := rc.DB.Raw(query, salonID, start, end, limit).Scan(&customers).Error
	return customers, err
}

func (rc *ReportController) getTopEmployees(salonID uuid.UUID, start, end time.Time, limit int) ([]EmployeeSummary, error) {
	var employees []EmployeeSummary

	query := `
		SELECT u.name, 
			   SUM(i.total) as revenue, 
			   COUNT(ii.id) as services_handled
		FROM invoices i
		INNER JOIN users u ON u.id = i.created_by_user_id
		LEFT JOIN invoice_items ii ON ii.invoice_id = i.id
		WHERE i.salon_id = ? 
		  AND i.invoice_date BETWEEN ? AND ?
		GROUP BY u.id, u.name
		ORDER BY revenue DESC
		LIMIT ?
	`

	err := rc.DB.Raw(query, salonID, start, end, limit).Scan(&employees).Error
	return employees, err
}

func (rc *ReportController) getEmployeeServiceDistribution(salonID uuid.UUID, start, end time.Time) ([]EmployeeServiceStats, error) {
	var stats []EmployeeServiceStats

	query := `
		SELECT u.name as employee_name, 
			   s.name as service_name, 
			   SUM(ii.quantity) as count, 
			   SUM(ii.total_price) as revenue
		FROM invoice_items ii
		INNER JOIN invoices i ON i.id = ii.invoice_id
		INNER JOIN users u ON u.id = i.created_by_user_id
		INNER JOIN services s ON s.id = ii.service_id
		WHERE i.salon_id = ? 
		  AND i.invoice_date BETWEEN ? AND ?
		GROUP BY u.id, u.name, s.id, s.name
		ORDER BY u.name, s.name
	`

	err := rc.DB.Raw(query, salonID, start, end).Scan(&stats).Error
	return stats, err
}

// Helper functions remain the same
func (rc *ReportController) getQuarterStart(date time.Time) time.Time {
	quarter := (int(date.Month())-1)/3 + 1
	startMonth := time.Month((quarter-1)*3 + 1)
	return time.Date(date.Year(), startMonth, 1, 0, 0, 0, 0, date.Location())
}

func (rc *ReportController) getQuarterEnd(date time.Time) time.Time {
	return rc.getQuarterStart(date).AddDate(0, 3, -1)
}

func (rc *ReportController) calculateGrowthPercentage(current, previous float64) float64 {
	if previous == 0 {
		if current == 0 {
			return 0
		}
		return 100
	}
	return ((current - previous) / previous) * 100
}
