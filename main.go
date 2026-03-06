package main

import (
	"salonpro-backend/config"
	"salonpro-backend/controllers"
	"salonpro-backend/routes"
	"salonpro-backend/services"
)

func main() {
	// Load Env File
	Env := config.LoadENV()

	//Db Connection Create
	db := config.ConnectDB(Env.DB_URL)

	// Auto migration Run
	config.AutoMigrationRun(db)

	// Handler SetUP
	handFunc := controllers.NewHandler(Env, db)

	otpCleanup := services.NewOTPCleanupService(db)

	// Start OTP cleanup scheduler in background — never delay server startup
	go func() {
		otpCleanup.Start()
	}()

	emailSvc := services.NewEmailService(Env)
	smsSvc := services.NewSMSService(Env)
	reminderSvc := services.NewReminderService(db, emailSvc, smsSvc)
	// Start birthday/anniversary reminder scheduler in background — never delay server startup
	go func() {
		reminderSvc.StartScheduler()
	}()

	port := Env.SERVER_PORT

	if port == "" {
		port = "8080"
	}
	r := routes.SetupRouter(handFunc)
	// printRoutes(r)
	r.Run(":" + port)
}
