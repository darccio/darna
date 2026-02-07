package models

import "example.com/testproject/helper"

// Response represents an API response.
type Response struct {
	Status  int
	Message string
}

// NewResponse creates a new response with formatted message.
func NewResponse(status int, msg string) *Response {
	return &Response{
		Status:  status,
		Message: helper.FormatMessage(msg),
	}
}
