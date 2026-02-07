package helper

// ValidateLength checks if a string meets minimum length.
func ValidateLength(s string, minLen int) bool {
	return len(s) >= minLen
}

// ValidatePositive checks if a number is positive.
func ValidatePositive(n int) bool {
	return n > 0
}
