package domain

import (
	"crypto/sha256"
	"encoding/json"
	"time"
)

// WebhookStatus is the lifecycle state of a webhook endpoint.
type WebhookStatus string

const (
	WebhookStatusActive   WebhookStatus = "active"
	WebhookStatusDisabled WebhookStatus = "disabled"
)

// Webhook represents a developer-configured HTTP endpoint that receives
// event notifications. The signing secret is stored as a SHA-256 hash;
// the plaintext is only returned once on creation.
type Webhook struct {
	ID            string
	ApplicationID string
	URL           string
	SecretHash    []byte
	Status        WebhookStatus
	Events        []string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// NewWebhook is the input for creating a webhook.
type NewWebhook struct {
	URL    string
	Events []string
}

// WebhookDeliveryStatus tracks the delivery lifecycle.
type WebhookDeliveryStatus string

const (
	WebhookDeliveryStatusPending    WebhookDeliveryStatus = "pending"
	WebhookDeliveryStatusDelivering WebhookDeliveryStatus = "delivering"
	WebhookDeliveryStatusDelivered  WebhookDeliveryStatus = "delivered"
	WebhookDeliveryStatusFailed     WebhookDeliveryStatus = "failed"
)

// WebhookDelivery is a single event delivery attempt record. The outbox
// pattern: events are written here first, then a background worker picks
// them up and delivers via HTTP POST with HMAC signature.
type WebhookDelivery struct {
	ID            string
	WebhookID     string
	EventType     string
	Payload       json.RawMessage
	Status        WebhookDeliveryStatus
	Attempts      int
	MaxAttempts   int
	NextAttemptAt time.Time
	LastError     string
	DeliveredAt   *time.Time
	CreatedAt     time.Time
}

// WebhookEvent is the payload envelope delivered to webhook endpoints.
type WebhookEvent struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	ApplicationID string          `json:"application_id"`
	CreatedAt     string          `json:"created_at"`
	Data          json.RawMessage `json:"data"`
}

// ValidWebhookEvents lists all event types that can be delivered via webhooks.
var ValidWebhookEvents = map[string]bool{
	"user.created":               true,
	"user.login.succeeded":       true,
	"user.login.failed":          true,
	"user.banned":                true,
	"user.unbanned":              true,
	"license.created":            true,
	"license.activated":          true,
	"license.expired":            true,
	"license.revoked":            true,
	"device.bound":               true,
	"device.changed":             true,
	"device.revoked":             true,
	"security.suspicious_device": true,
}

// HashWebhookSecret returns the SHA-256 of the plaintext webhook signing
// secret for storage. The plaintext is only returned to the API consumer
// once on creation.
func HashWebhookSecret(secret []byte) []byte {
	digest := sha256.Sum256(secret)
	return digest[:]
}

// MatchWebhookEvent reports whether the given event type matches any of
// the webhook's subscribed patterns. Patterns can be exact matches or
// use '*' as a wildcard suffix (e.g., "license.*" matches "license.created").
func MatchWebhookEvent(eventType string, patterns []string) bool {
	for _, pattern := range patterns {
		if pattern == "*" || pattern == eventType {
			return true
		}
		// Wildcard suffix: "license.*" matches "license.created"
		if len(pattern) > 2 && pattern[len(pattern)-1] == '*' && pattern[len(pattern)-2] == '.' {
			prefix := pattern[:len(pattern)-1]
			if len(eventType) >= len(prefix) && eventType[:len(prefix)] == prefix {
				return true
			}
		}
	}
	return false
}
