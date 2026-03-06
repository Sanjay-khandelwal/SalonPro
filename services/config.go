package services

import "salonpro-backend/config"

type Services struct {
	Env           *config.ENV
	TwilioService *TwilioService
	ResendService *ResendService
}

func NewServices(env *config.ENV) *Services {

	return &Services{
		Env:           env,
		TwilioService: NewSMSService(env),
		ResendService: NewEmailService(env),
	}
}
