package main

import (
	"example.com/testproject/helper"
	"example.com/testproject/models"
)

// ProcessRequest processes a request and returns a response.
func ProcessRequest(req *models.Request) *models.Response {
	if !helper.ValidatePositive(req.ID) {
		return models.NewResponse(400, "Invalid ID")
	}
	if !helper.ValidateLength(req.Payload, 1) {
		return models.NewResponse(400, "Empty payload")
	}
	return models.NewResponse(200, "Success")
}

// CreateAndProcess creates a new request and processes it.
func CreateAndProcess(id int, payload string) *models.Response {
	req := models.NewRequest(id, payload)
	return ProcessRequest(req)
}
