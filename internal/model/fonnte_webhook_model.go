package model
 
// FonnteWebhookPayload is the incoming payload from Fonnte's webhook.
// Fields marked "full-feature" are only sent by devices on the complete Fonnte package.
type FonnteWebhookPayload struct {
	Device  string `json:"device" form:"device"`
	Sender  string `json:"sender" form:"sender"`   // Phone number (e.g. 6281234567890)
	Message string `json:"message" form:"message"` // Text content
	Name    string `json:"name" form:"name"`       // Sender's WA display name
	Member  string `json:"member" form:"member"`   // Group member (empty for personal)
	Location string `json:"location" form:"location"` // Shared location string
	// Media fields — received when sender attaches a file
	File      string `json:"file" form:"file"`           // URL of received file/image
	MimeType  string `json:"mime_type" form:"mime_type"` // e.g. image/jpeg
	// Full-feature device fields only
	URL       string `json:"url" form:"url"`             // Media URL (full-feature)
	Filename  string `json:"filename" form:"filename"`   // Original filename (full-feature)
	Extension string `json:"extension" form:"extension"` // File extension, e.g. "jpg" (full-feature)
}
 
// IsImageMessage returns true if the message contains an image.
func (p *FonnteWebhookPayload) IsImageMessage() bool {
	return p.File != "" &&
		(p.MimeType == "image/jpeg" || p.MimeType == "image/png" || p.MimeType == "image/webp")
}

// IsGroupMessage returns true when the message comes from a group chat.
func (p *FonnteWebhookPayload) IsGroupMessage() bool {
	return p.Member != ""
}

// ToIncomingMessage converts the Fonnte-specific payload to the generic
// IncomingMessage used throughout the rest of the system.
func (p *FonnteWebhookPayload) ToIncomingMessage() IncomingMessage {
	return IncomingMessage{
		Platform:      PlatformFonnte,
		Sender:        p.Sender,
		SenderName:    p.Name,
		Message:       p.Message,
		AttachmentURL: p.File,
		MimeType:      p.MimeType,
		IsGroup:       p.IsGroupMessage(),
	}
}