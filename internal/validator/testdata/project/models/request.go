package models

// Request represents an API request.
type Request struct {
	ID      int
	Payload string
}

// NewRequest creates a new request.
func NewRequest(id int, payload string) *Request {
	return &Request{
		ID:      id,
		Payload: payload,
	}
}
