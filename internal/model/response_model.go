package model

import "time"

// PaginationMeta represents server-side pagination metadata.
type PaginationMeta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// PaginatedResponse wraps list responses with pagination metadata.
type PaginatedResponse struct {
	Data interface{}    `json:"data"`
	Meta PaginationMeta `json:"meta"`
}

// SendMessageRequest is the payload sent to Fonnte API.
type SendMessageRequest struct {
	Target  string `json:"target"`          // Recipient phone number
	Message string `json:"message"`         // Text to send
	Delay   int    `json:"delay,omitempty"` // Delay in seconds
}

// WorkerJob is a unit of async work dispatched to the worker pool.
// Payload is a provider-agnostic IncomingMessage so the pool can handle
// messages from both Fonnte and Telegram without any changes.
type WorkerJob struct {
	Payload  IncomingMessage
	Session  *Session
	RecvedAt time.Time
}