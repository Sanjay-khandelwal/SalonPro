// utils/validation.go
package utils

import (
	"regexp"
	"strings"
)

// ValidatePhone checks if a phone number is in a valid international format
// ValidateIndianPhone validates Indian mobile numbers
func ValidatePhone(phone string) bool {
	// Remove spaces, dashes, parentheses
	cleaned := strings.ReplaceAll(phone, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, "(", "")
	cleaned = strings.ReplaceAll(cleaned, ")", "")

	// Regex for Indian numbers:
	// Optional +91 at start, then first digit 6-9, then 9 digits
	regex := `^(\+91)?[6-9]\d{9}$`

	matched, err := regexp.MatchString(regex, cleaned)
	if err != nil {
		return false
	}
	return matched
}
