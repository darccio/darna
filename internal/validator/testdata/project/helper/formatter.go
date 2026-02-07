package helper

import "fmt"

// FormatMessage formats a message with a prefix.
func FormatMessage(msg string) string {
	return fmt.Sprintf("[HELPER] %s", msg)
}

// FormatNumber formats a number as a string.
func FormatNumber(n int) string {
	return fmt.Sprintf("#%d", n)
}
