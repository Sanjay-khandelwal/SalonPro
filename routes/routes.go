package routes

import (
	"salonpro-backend/config"
	"salonpro-backend/controllers"
	"salonpro-backend/utils"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func SetupRouter(handFunc *controllers.HandlerFunc) *gin.Engine {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{
			"https://white-sky-0debbc31e.1.azurestaticapps.net",
			"https://salon.zenithive.digital",
			"https://salonpro.zenithive.digital",
			"https://salon-pro.netlify.app",
			"http://localhost:3000",
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	r.Use(config.PerformanceLogger())

	auth := r.Group("/auth")
	{
		auth.POST("/register", handFunc.Register)
		auth.POST("/registerOTP", handFunc.RegisterOTP)
		auth.POST("/login", handFunc.Login)

		// Public: forgot password flow (no auth required)
		auth.POST("/forgot-password", handFunc.ForgotPassword)
		auth.POST("/resend-otp", handFunc.ResendOTP)
		auth.POST("/verify-otp", handFunc.VerifyOTP)
		auth.POST("/reset-password", handFunc.ResetPassword)

		auth.Use(utils.AuthMiddleware())

		auth.GET("/me", handFunc.Me)
		auth.PUT("/me", handFunc.UpdateUserProfile)
		auth.POST("/me/change-password", handFunc.ChangePassword)

	}

	api := r.Group("/api")
	api.Use(utils.AuthMiddleware())
	{
		// Customer routes
		customers := api.Group("/customers")
		{
			customers.POST("", handFunc.CreateCustomer)
			customers.GET("", handFunc.GetCustomers)
			customers.GET("/:id", handFunc.GetCustomer)
			customers.PUT("/:id", handFunc.UpdateCustomer)
			customers.DELETE("/:id", handFunc.DeleteCustomer)
		}

		// Service routes
		services := api.Group("/services")
		{
			services.POST("", handFunc.CreateService)
			services.GET("", handFunc.GetServices)
			services.GET("/:id", handFunc.GetService)
			services.PUT("/:id", handFunc.UpdateService)
			services.DELETE("/:id", handFunc.DeleteService)
		}

		// Payment methods (for invoice create/edit)
		api.GET("/payment-methods", handFunc.GetPaymentMethods)

		// Invoice routes
		invoices := api.Group("/invoices")
		{
			invoices.POST("", handFunc.CreateInvoice)
			invoices.GET("", handFunc.GetInvoices)
			invoices.GET("/pdf", handFunc.GetInvoicePDF) // must be before /:id
			invoices.GET("/:id", handFunc.GetInvoice)
			invoices.PUT("/:id", handFunc.UpdateInvoice)
			invoices.DELETE("/:id", handFunc.DeleteInvoice)
		}

		//Reports routes
		reportController := controllers.NewReportAnalysis(handFunc.DB)
		api.GET("/reports", reportController.GetReportAnalytics)

		// Dashboard routes
		api.GET("/dashboard", handFunc.GetDashboardOverview)

		// Settings routes
		profile := auth.Group("/profile", utils.AuthMiddleware()) // utils.AuthMiddleware()
		{
			profile.GET("", handFunc.GetProfile)
			profile.PUT("/update-salon", handFunc.UpdateSalonProfile)
			profile.PUT("/update-hours", handFunc.UpdateWorkingHours)
			profile.PUT("/update-templates", handFunc.UpdateReminderTemplates)
			profile.PUT("/update-notifications", handFunc.UpdateNotifications)
		}

		employees := api.Group("/employees")
		{
			employees.GET("", handFunc.GetEmployees)          // GET /api/employees
			employees.POST("", handFunc.AddEmployee)          // POST /api/employees
			employees.PUT("/:id", handFunc.UpdateEmployee)    // PUT /api/employees/:id
			employees.DELETE("/:id", handFunc.DeleteEmployee) // DELETE /api/employees/:id
		}

	}

	return r
}
