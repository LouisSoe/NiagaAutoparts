package model

import "context"

// Platform identifies which messaging provider delivered the message.
type Platform string

const (
	PlatformFonnte   Platform = "fonnte"
	PlatformTelegram Platform = "telegram"
)

// IncomingMessage is the normalised, provider-agnostic representation of any
// inbound message. Both the Fonnte webhook and the Telegram webhook map their
// raw payloads into this struct before handing the job to the worker pool.
type IncomingMessage struct {
	// Platform that delivered this message.
	Platform Platform

	// Sender is the canonical "address" used when replying:
	//   Fonnte   → phone number, e.g. "6281234567890"
	//   Telegram → chat ID as a string, e.g. "123456789"
	Sender string

	// SenderName is the human-readable display name (best-effort, may be empty).
	SenderName string

	// Message is the plain text body of the message.
	Message string

	// AttachmentURL is a publicly accessible URL of an attached file/image.
	// Empty when there is no attachment.
	AttachmentURL string

	// MimeType hints the content type of AttachmentURL, e.g. "image/jpeg".
	MimeType string

	// IsGroup is true when the message was sent inside a group chat.
	IsGroup bool
}

// IsImageMessage returns true when the message contains an image attachment.
func (m *IncomingMessage) IsImageMessage() bool {
	return m.AttachmentURL != "" &&
		(m.MimeType == "image/jpeg" || m.MimeType == "image/png" ||
			m.MimeType == "image/webp" || m.MimeType == "image/gif")
}

// IsFileMessage returns true when there is a non-image file attachment.
func (m *IncomingMessage) IsFileMessage() bool {
	return m.AttachmentURL != "" && !m.IsImageMessage()
}

// MessageSender is implemented by any provider that can deliver outgoing
// messages. Both MessagingService (Fonnte) and TelegramService implement this
// interface so that MessageProcessor and the worker pool stay provider-agnostic.
type MessageSender interface {
	// SendText delivers a plain-text message to the given recipient address.
	// The platform argument lets the composite sender route to the right provider.
	SendText(ctx context.Context, platform Platform, to, message string) error

	// SendMedia delivers a message with an attached file.
	//   caption   – optional text to accompany the file
	//   mediaURL  – publicly accessible URL of the file
	//   filename  – display name (Fonnte uses this; Telegram ignores it)
	//   mimeType  – optional MIME hint, e.g. "image/jpeg"
	SendMedia(ctx context.Context, platform Platform, to, caption, mediaURL, filename, mimeType string) error
}
