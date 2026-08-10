package database

import "time"

// Account represents a tenant in the messaging service.
type Account struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email"`
	Plan        string    `json:"plan"`
	QueueLimit  int       `json:"queue_limit"`
	UsedQueues  int       `json:"used_queues"`
	CreatedAt   time.Time `json:"created_at"`
	Status      string    `json:"status"`
}

// Project represents a named namespace of messaging resources within an account.
type Project struct {
	ID          string    `json:"id"`
	AccountID   string    `json:"account_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	QueueCount  int       `json:"queue_count"`
	TopicCount  int       `json:"topic_count"`
	CreatedAt   time.Time `json:"created_at"`
	Status      string    `json:"status"`
}

// Queue represents an SQS-style message queue.
type Queue struct {
	ID                   string    `json:"id"`
	ProjectID            string    `json:"project_id"`
	Name                 string    `json:"name"`
	VisibilityTimeoutSec int       `json:"visibility_timeout_sec"`
	MessageRetentionSec  int       `json:"message_retention_sec"`
	State                string    `json:"state"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
	Status               string    `json:"status"`
}

// QueueMessage represents a single message sitting in a queue.
type QueueMessage struct {
	ID         string    `json:"id"`
	QueueID    string    `json:"queue_id"`
	Body       string    `json:"body"`
	Attributes []byte    `json:"attributes,omitempty"`
	State      string    `json:"state"`
	VisibleAt  time.Time `json:"visible_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// Topic represents an SNS-style pub-sub topic.
type Topic struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Status    string    `json:"status"`
}

// Subscriber represents a queue or webhook attached to a topic.
type Subscriber struct {
	ID         string    `json:"id"`
	TopicID    string    `json:"topic_id"`
	Kind       string    `json:"kind"`
	QueueID    string    `json:"queue_id,omitempty"`
	QueueName  string    `json:"queue_name,omitempty"`
	WebhookURL string    `json:"webhook_url,omitempty"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// WebhookDelivery represents one HTTP push attempt result for a published message.
type WebhookDelivery struct {
	ID           int64     `json:"id"`
	SubscriberID string    `json:"subscriber_id"`
	MessageID    string    `json:"message_id"`
	Payload      string    `json:"payload"`
	Status       string    `json:"status"`
	Attempts     int       `json:"attempts"`
	LastError    string    `json:"last_error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
