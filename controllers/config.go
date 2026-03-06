package controllers

import (
	"salonpro-backend/config"
	"salonpro-backend/services"

	"gorm.io/gorm"
)

type HandlerFunc struct {
	Env     *config.ENV
	DB      *gorm.DB
	Service *services.Services
}

// NewHandler initializes and returns a HandlerFunc
func NewHandler(env *config.ENV, db *gorm.DB) *HandlerFunc {
	services := services.NewServices(env)
	return &HandlerFunc{
		Env:     env,
		DB:      db,
		Service: services,
	}
}
