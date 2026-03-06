package services

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
)

// GenerateOTP returns a 6-digit numeric OTP.
func (s *Services) GenerateOTP() (string, error) {
	max := big.NewInt(1000000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func (s *Services) VerifyTurnstile(token string) bool {

	resp, err := http.PostForm(
		"https://challenges.cloudflare.com/turnstile/v0/siteverify",
		url.Values{
			"secret":   {s.Env.TURNSTILE_SECRET},
			"response": {token},
		},
	)
	if err != nil {
		fmt.Println("failde to verify", err.Error())
		return false
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
	}

	json.NewDecoder(resp.Body).Decode(&result)
	return result.Success
}
