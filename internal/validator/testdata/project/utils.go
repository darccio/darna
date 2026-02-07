package main

import "fmt"

// FormatUser formats a user for display.
func FormatUser(u User) string {
	return fmt.Sprintf("User[%d]: %s", u.ID, u.Name)
}

// ValidateConfig validates configuration.
func ValidateConfig(c Config) bool {
	return c.Port > 0 && c.Timeout > 0
}

// Helper is a simple helper function.
func Helper() string {
	return "helper"
}
