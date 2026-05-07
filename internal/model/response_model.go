package model

import "time"

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