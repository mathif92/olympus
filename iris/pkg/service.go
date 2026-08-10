// Package pkg contains the Iris business-logic layer: multi-tenant messaging,
// with SQS-style queues (visibility + retention), SNS-style topics, topic
// subscribers (queue fan-out + webhook HTTP push) and audit trails.
package pkg

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mathif92/olympus/iris/pkg/database"
)

// Audit operations recorded in the audit_logs table.
const (
	OpCreate      = "create"
	OpList        = "list"
	OpDelete      = "delete"
	OpSend        = "send"
	OpPoll        = "poll"
	OpAck         = "ack"
	OpPublish     = "publish"
	OpSubscribe   = "subscribe"
	OpUnsubscribe = "unsubscribe"
)

// Subscriber kinds.
const (
	SubscriberQueue   = "queue"
	SubscriberWebhook = "webhook"
)

// Message states.
const (
	MsgPending   = "pending"
	MsgInFlight  = "in_flight"
	MsgDelivered = "delivered"
)

// ErrNotFound is returned when a requested project or resource does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when an operation cannot complete from the current state.
var ErrConflict = errors.New("state conflict")

// Iris is the messaging control-plane orchestrator.
type Iris struct {
	DB         *database.Client
	HTTPClient *http.Client
}

// NewIris wires the messaging control plane to Postgres.
func NewIris(db *database.Client) *Iris {
	return &Iris{
		DB:         db,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// EnsureAccount upserts the tenant so requests referencing it always resolve.
func (i *Iris) EnsureAccount(ctx context.Context, a database.Account) error {
	_, err := i.DB.Exec(ctx, `
		INSERT INTO accounts (id, display_name, email, plan, queue_limit)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET display_name = EXCLUDED.display_name`,
		a.ID, a.DisplayName, a.Email, a.Plan, a.QueueLimit)
	return err
}

// CreateProject provisions a project namespace inside the account.
func (i *Iris) CreateProject(ctx context.Context, accountID string, p database.Project) error {
	if p.Name == "" {
		return errors.New("project name is required")
	}
	p.ID = newID()
	_, err := i.DB.Exec(ctx, `
		INSERT INTO projects (id, account_id, name, description)
		VALUES ($1, $2, $3, $4)`,
		p.ID, accountID, p.Name, p.Description)
	return err
}

// ListProjects returns all projects for an account.
func (i *Iris) ListProjects(ctx context.Context, accountID string) ([]database.Project, error) {
	rows, err := i.DB.Query(ctx, `
		SELECT id, account_id, name, COALESCE(description,''), queue_count, topic_count, created_at, status
		FROM projects WHERE account_id = $1 ORDER BY name`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.Project
	for rows.Next() {
		var p database.Project
		if err := rows.Scan(&p.ID, &p.AccountID, &p.Name, &p.Description, &p.QueueCount, &p.TopicCount, &p.CreatedAt, &p.Status); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (i *Iris) resolveProject(ctx context.Context, accountID, projectName string) (*database.Project, error) {
	var p database.Project
	err := i.DB.QueryRow(ctx, `
		SELECT id, account_id, name, COALESCE(description,''), queue_count, topic_count, created_at, status
		FROM projects WHERE account_id = $1 AND name = $2 AND status = 'active'`,
		accountID, projectName).Scan(
		&p.ID, &p.AccountID, &p.Name, &p.Description, &p.QueueCount, &p.TopicCount, &p.CreatedAt, &p.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// IsNotFound reports whether an error wraps sql.ErrNoRows / ErrNotFound.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	return err == ErrNotFound || errors.Is(err, sql.ErrNoRows)
}

// --- Queues ---

// CreateQueue creates an SQS-style queue in a project.
func (i *Iris) CreateQueue(ctx context.Context, accountID, projectName string, q database.Queue) (*database.Queue, error) {
	proj, err := i.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	if q.Name == "" {
		return nil, errors.New("queue name is required")
	}
	if q.VisibilityTimeoutSec <= 0 {
		q.VisibilityTimeoutSec = 30
	}
	if q.MessageRetentionSec <= 0 {
		q.MessageRetentionSec = 86400
	}
	q.ID = proj.ID + "-" + q.Name
	q.ProjectID = proj.ID
	q.State = "active"
	if _, err := i.DB.Exec(ctx, `
		INSERT INTO queues (id, project_id, name, visibility_timeout_sec, message_retention_sec, state)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		q.ID, proj.ID, q.Name, q.VisibilityTimeoutSec, q.MessageRetentionSec, q.State); err != nil {
		return nil, err
	}
	i.refreshCounts(ctx, proj.ID)
	return i.GetQueue(ctx, accountID, projectName, q.Name)
}

// GetQueue returns a queue and its live message counts.
func (i *Iris) GetQueue(ctx context.Context, accountID, projectName, name string) (*database.Queue, error) {
	proj, err := i.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	q := &database.Queue{}
	err = i.DB.QueryRow(ctx, `
		SELECT id, project_id, name, visibility_timeout_sec, message_retention_sec, state, created_at, updated_at, status
		FROM queues WHERE project_id = $1 AND name = $2`,
		proj.ID, name).Scan(
		&q.ID, &q.ProjectID, &q.Name, &q.VisibilityTimeoutSec, &q.MessageRetentionSec, &q.State, &q.CreatedAt, &q.UpdatedAt, &q.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return q, nil
}

// ListQueues returns all queues in a project.
func (i *Iris) ListQueues(ctx context.Context, accountID, projectName string) ([]database.Queue, error) {
	proj, err := i.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	rows, err := i.DB.Query(ctx, `
		SELECT id, project_id, name, visibility_timeout_sec, message_retention_sec, state, created_at, updated_at, status
		FROM queues WHERE project_id = $1 ORDER BY name`, proj.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.Queue
	for rows.Next() {
		var q database.Queue
		if err := rows.Scan(&q.ID, &q.ProjectID, &q.Name, &q.VisibilityTimeoutSec, &q.MessageRetentionSec, &q.State, &q.CreatedAt, &q.UpdatedAt, &q.Status); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// DeleteQueue removes a queue and cascades its messages; any topic subscribers
// pointing at it are removed too.
func (i *Iris) DeleteQueue(ctx context.Context, accountID, projectName, name string) error {
	proj, err := i.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return err
	}
	_, err = i.DB.Exec(ctx, `
		DELETE FROM topic_subscribers WHERE queue_id = (SELECT id FROM queues WHERE project_id = $1 AND name = $2)`,
		proj.ID, name)
	if err != nil {
		return err
	}
	_, err = i.DB.Exec(ctx, `
		DELETE FROM queues WHERE project_id = $1 AND name = $2`,
		proj.ID, name)
	if err != nil {
		return err
	}
	i.refreshCounts(ctx, proj.ID)
	return nil
}

// SendMessage enqueues a message into a queue (immediately visible).
func (i *Iris) SendMessage(ctx context.Context, accountID, projectName, queueName, body string, attrs map[string]string) (*database.QueueMessage, error) {
	q, err := i.GetQueue(ctx, accountID, projectName, queueName)
	if err != nil {
		return nil, err
	}
	msg := &database.QueueMessage{
		ID:         newID(),
		QueueID:    q.ID,
		Body:       body,
		Attributes: []byte("{}"),
	}
	if len(attrs) > 0 {
		b, err := json.Marshal(attrs)
		if err != nil {
			return nil, err
		}
		msg.Attributes = b
	}
	_, err = i.DB.Exec(ctx, `
		INSERT INTO queue_messages (id, queue_id, body, attributes, state, visible_at, expires_at)
		VALUES ($1, $2, $3, $4, 'pending', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP + ($5 * INTERVAL '1 second'))`,
		msg.ID, q.ID, body, msg.Attributes, q.MessageRetentionSec)
	if err != nil {
		return nil, err
	}
	return i.getMessage(ctx, msg.ID)
}

// PollMessages returns up to maxMessages visible messages from a queue, marking
// them in_flight with a visibility window (SQS semantics).
func (i *Iris) PollMessages(ctx context.Context, accountID, projectName, queueName string, maxMessages int) ([]database.QueueMessage, error) {
	q, err := i.GetQueue(ctx, accountID, projectName, queueName)
	if err != nil {
		return nil, err
	}
	if maxMessages <= 0 {
		maxMessages = 1
	}
	if maxMessages > 10 {
		maxMessages = 10
	}

	tx, err := i.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// Re-expose any in_flight messages whose visibility window has elapsed.
	if _, err := tx.ExecContext(ctx, `
		UPDATE queue_messages SET state = 'pending', visible_at = CURRENT_TIMESTAMP
		WHERE queue_id = $1 AND state = 'in_flight' AND visible_at <= CURRENT_TIMESTAMP`, q.ID); err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT id, queue_id, COALESCE(body,''), COALESCE(attributes,'{}'::jsonb), state, visible_at, created_at
		FROM queue_messages
		WHERE queue_id = $1 AND state = 'pending' AND visible_at <= CURRENT_TIMESTAMP
		ORDER BY created_at LIMIT $2
		FOR UPDATE`, q.ID, maxMessages)
	if err != nil {
		return nil, err
	}
	var msgs []database.QueueMessage
	for rows.Next() {
		var m database.QueueMessage
		if err := rows.Scan(&m.ID, &m.QueueID, &m.Body, &m.Attributes, &m.State, &m.VisibleAt, &m.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		msgs = append(msgs, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range msgs {
		if _, err := tx.ExecContext(ctx, `
			UPDATE queue_messages SET state = 'in_flight',
			        visible_at = CURRENT_TIMESTAMP + ($2 * INTERVAL '1 second')
			WHERE id = $1`, msgs[i].ID, q.VisibilityTimeoutSec); err != nil {
			return nil, err
		}
		msgs[i].State = "in_flight"
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return msgs, nil
}

// AckMessage acknowledges (removes) a delivered message by id.
func (i *Iris) AckMessage(ctx context.Context, accountID, projectName, queueName, messageID string) error {
	q, err := i.GetQueue(ctx, accountID, projectName, queueName)
	if err != nil {
		return err
	}
	_, err = i.DB.Exec(ctx, `
		DELETE FROM queue_messages WHERE id = $1 AND queue_id = $2 AND state = 'in_flight'`,
		messageID, q.ID)
	if err != nil {
		return err
	}
	return nil
}

func (i *Iris) getMessage(ctx context.Context, id string) (*database.QueueMessage, error) {
	var m database.QueueMessage
	err := i.DB.QueryRow(ctx, `
		SELECT id, queue_id, COALESCE(body,''), COALESCE(attributes,'{}'::jsonb), state, visible_at, created_at
		FROM queue_messages WHERE id = $1`, id).Scan(
		&m.ID, &m.QueueID, &m.Body, &m.Attributes, &m.State, &m.VisibleAt, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// --- Topics ---

// CreateTopic creates an SNS-style topic in a project.
func (i *Iris) CreateTopic(ctx context.Context, accountID, projectName string, t database.Topic) (*database.Topic, error) {
	proj, err := i.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	if t.Name == "" {
		return nil, errors.New("topic name is required")
	}
	t.ID = proj.ID + "-" + t.Name
	t.ProjectID = proj.ID
	t.State = "active"
	if _, err := i.DB.Exec(ctx, `
		INSERT INTO topics (id, project_id, name, state)
		VALUES ($1, $2, $3, $4)`,
		t.ID, proj.ID, t.Name, t.State); err != nil {
		return nil, err
	}
	i.refreshCounts(ctx, proj.ID)
	return i.GetTopic(ctx, accountID, projectName, t.Name)
}

// GetTopic returns a topic and its subscriber count.
func (i *Iris) GetTopic(ctx context.Context, accountID, projectName, name string) (*database.Topic, error) {
	proj, err := i.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	t := &database.Topic{}
	err = i.DB.QueryRow(ctx, `
		SELECT id, project_id, name, state, created_at, updated_at, status
		FROM topics WHERE project_id = $1 AND name = $2`,
		proj.ID, name).Scan(
		&t.ID, &t.ProjectID, &t.Name, &t.State, &t.CreatedAt, &t.UpdatedAt, &t.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// ListTopics returns all topics in a project.
func (i *Iris) ListTopics(ctx context.Context, accountID, projectName string) ([]database.Topic, error) {
	proj, err := i.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	rows, err := i.DB.Query(ctx, `
		SELECT id, project_id, name, state, created_at, updated_at, status
		FROM topics WHERE project_id = $1 ORDER BY name`, proj.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.Topic
	for rows.Next() {
		var t database.Topic
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Name, &t.State, &t.CreatedAt, &t.UpdatedAt, &t.Status); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteTopic removes a topic, its subscribers, and delivery history.
func (i *Iris) DeleteTopic(ctx context.Context, accountID, projectName, name string) error {
	proj, err := i.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return err
	}
	_, err = i.DB.Exec(ctx, `DELETE FROM topics WHERE project_id = $1 AND name = $2`, proj.ID, name)
	if err != nil {
		return err
	}
	i.refreshCounts(ctx, proj.ID)
	return nil
}

// --- Subscribers ---

// SubscribeQueue attaches a queue to a topic: published messages are fanned out
// as new messages into the queue.
func (i *Iris) SubscribeQueue(ctx context.Context, accountID, projectName, topicName, queueName string) (*database.Subscriber, error) {
	proj, err := i.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	queue, err := i.getQueueInProject(ctx, proj.ID, queueName)
	if err != nil {
		return nil, err
	}
	topic, err := i.getTopicInProject(ctx, proj.ID, topicName)
	if err != nil {
		return nil, err
	}
	s := &database.Subscriber{
		ID:      newID(),
		TopicID: topic.ID,
		Kind:    SubscriberQueue,
		QueueID: queue.ID,
	}
	_, err = i.DB.Exec(ctx, `
		INSERT INTO topic_subscribers (id, topic_id, kind, queue_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (topic_id, queue_id) DO NOTHING`,
		s.ID, topic.ID, SubscriberQueue, queue.ID)
	if err != nil {
		return nil, err
	}
	return i.getSubscriber(ctx, s.ID)
}

// SubscribeWebhook attaches a webhook URL to a topic: every published message
// is POSTed to the URL (with retries).
func (i *Iris) SubscribeWebhook(ctx context.Context, accountID, projectName, topicName, webhookURL string) (*database.Subscriber, error) {
	proj, err := i.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	topic, err := i.getTopicInProject(ctx, proj.ID, topicName)
	if err != nil {
		return nil, err
	}
	s := &database.Subscriber{
		ID:         newID(),
		TopicID:    topic.ID,
		Kind:       SubscriberWebhook,
		WebhookURL: webhookURL,
	}
	_, err = i.DB.Exec(ctx, `
		INSERT INTO topic_subscribers (id, topic_id, kind, webhook_url)
		VALUES ($1, $2, $3, $4)`,
		s.ID, topic.ID, SubscriberWebhook, webhookURL)
	if err != nil {
		return nil, err
	}
	return i.getSubscriber(ctx, s.ID)
}

func (i *Iris) getSubscriber(ctx context.Context, id string) (*database.Subscriber, error) {
	s := &database.Subscriber{}
	err := i.DB.QueryRow(ctx, `
		SELECT id, topic_id, kind, COALESCE(queue_id, ''), COALESCE(webhook_url, ''),
		       status, created_at, updated_at
		FROM topic_subscribers WHERE id = $1`, id).Scan(
		&s.ID, &s.TopicID, &s.Kind, &s.QueueID, &s.WebhookURL, &s.Status, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if s.QueueID != "" {
		if err := i.DB.QueryRow(ctx, `SELECT name FROM queues WHERE id = $1`, s.QueueID).Scan(&s.QueueName); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// ListSubscribers returns all subscribers attached to a topic.
func (i *Iris) ListSubscribers(ctx context.Context, accountID, projectName, topicName string) ([]database.Subscriber, error) {
	proj, err := i.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	topic, err := i.getTopicInProject(ctx, proj.ID, topicName)
	if err != nil {
		return nil, err
	}
	rows, err := i.DB.Query(ctx, `
		SELECT s.id, s.topic_id, s.kind,
		       COALESCE(s.queue_id, ''), COALESCE(s.webhook_url, ''), s.status, s.created_at, s.updated_at
		FROM topic_subscribers s WHERE s.topic_id = $1 ORDER BY s.created_at`, topic.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.Subscriber
	for rows.Next() {
		var s database.Subscriber
		if err := rows.Scan(&s.ID, &s.TopicID, &s.Kind, &s.QueueID, &s.WebhookURL, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		if s.QueueID != "" {
			var qname string
			if err := i.DB.QueryRow(ctx, `SELECT name FROM queues WHERE id = $1`, s.QueueID).Scan(&qname); err == nil {
				s.QueueName = qname
			}
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Unsubscribe removes a subscriber from a topic.
func (i *Iris) Unsubscribe(ctx context.Context, accountID, projectName, topicName, subscriberID string) error {
	proj, err := i.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return err
	}
	topic, err := i.getTopicInProject(ctx, proj.ID, topicName)
	if err != nil {
		return err
	}
	_, err = i.DB.Exec(ctx, `
		DELETE FROM topic_subscribers WHERE id = $1 AND topic_id = $2`,
		subscriberID, topic.ID)
	if err != nil {
		return err
	}
	return nil
}

// PublishMessage fans a message out to every subscriber: queue subscribers get
// a real enqueued copy; webhook subscribers get a real HTTP POST (with retries).
// It returns the number of queue copies and webhook deliveries created.
func (i *Iris) PublishMessage(ctx context.Context, accountID, projectName, topicName, body string) (queueCopies, webhookDeliveries int, err error) {
	proj, err := i.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return 0, 0, err
	}
	topic, err := i.getTopicInProject(ctx, proj.ID, topicName)
	if err != nil {
		return 0, 0, err
	}

	rows, err := i.DB.Query(ctx, `
		SELECT id, kind, COALESCE(queue_id, ''), COALESCE(webhook_url, '')
		FROM topic_subscribers WHERE topic_id = $1 AND status = 'active'`, topic.ID)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	messageID := newID()
	var queueSubs, webhookSubs []database.Subscriber
	for rows.Next() {
		var s database.Subscriber
		if err := rows.Scan(&s.ID, &s.Kind, &s.QueueID, &s.WebhookURL); err != nil {
			return 0, 0, err
		}
		if s.Kind == SubscriberQueue {
			queueSubs = append(queueSubs, s)
		} else {
			webhookSubs = append(webhookSubs, s)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	// Fan out into each subscribed queue with the topic's message id as origin.
	for _, s := range queueSubs {
		if _, err := i.DB.Exec(ctx, `
			INSERT INTO queue_messages (id, queue_id, body, attributes, state, visible_at, expires_at)
			VALUES ($1, $2, $3, $4, 'pending', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP + INTERVAL '1 day')`,
			newID(), s.QueueID, body, []byte(`{"origin":"topic"}`)); err != nil {
			return 0, 0, err
		}
		queueCopies++
	}

	// Push to each webhook subscriber (synchronous, with retries).
	for _, s := range webhookSubs {
		deliveryID, err := i.recordDelivery(ctx, s.ID, messageID, body)
		if err != nil {
			return 0, 0, err
		}
		delivered, err := i.deliverWebhook(ctx, s.ID, deliveryID, s.WebhookURL, body)
		if err != nil {
			_ = err
		}
		if delivered {
			webhookDeliveries++
		}
	}
	return queueCopies, webhookDeliveries, nil
}

func (i *Iris) recordDelivery(ctx context.Context, subscriberID, messageID, payload string) (int64, error) {
	var id int64
	err := i.DB.QueryRow(ctx, `
		INSERT INTO webhook_deliveries (subscriber_id, message_id, payload, status)
		VALUES ($1, $2, $3, 'pending')
		RETURNING id`,
		subscriberID, messageID, payload).Scan(&id)
	return id, err
}

// deliverWebhook POSTs the message body to the subscriber URL with retries and
// records each attempt. Returns true once the endpoint accepts the message.
func (i *Iris) deliverWebhook(ctx context.Context, subscriberID string, deliveryID int64, url, body string) (bool, error) {
	const maxAttempts = 3
	delay := 100 * time.Millisecond
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := i.HTTPClient.Post(url, "application/json", bytes.NewBufferString(body))
		status := "failed"
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				status = "delivered"
			}
		}
		lastErr := ""
		if err != nil {
			lastErr = err.Error()
		}
		_, _ = i.DB.Exec(ctx, `
			UPDATE webhook_deliveries
			SET status = $2, attempts = $3, last_error = NULLIF($4, ''), updated_at = CURRENT_TIMESTAMP
			WHERE id = $1`,
			deliveryID, status, attempt, lastErr)
		if status == "delivered" {
			return true, nil
		}
		if attempt < maxAttempts {
			time.Sleep(delay)
			delay *= 2
		}
	}
	return false, fmt.Errorf("webhook delivery exhausted %d attempts", maxAttempts)
}

// Audit records an operation in the audit trail.
func (i *Iris) Audit(ctx context.Context, accountID, projectID, entity, operation, status string) error {
	_, err := i.DB.Exec(ctx, `
		INSERT INTO audit_logs (account_id, project_id, entity, operation, status)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5)`,
		accountID, projectID, entity, operation, status)
	return err
}

func (i *Iris) refreshCounts(ctx context.Context, projectID string) {
	_, _ = i.DB.Exec(ctx, `
		UPDATE projects SET
			queue_count = (SELECT COUNT(*) FROM queues WHERE project_id = $1),
			topic_count = (SELECT COUNT(*) FROM topics WHERE project_id = $1)
		WHERE id = $1`, projectID)
}

func (i *Iris) getQueueInProject(ctx context.Context, projectID, name string) (*database.Queue, error) {
	q := &database.Queue{}
	err := i.DB.QueryRow(ctx, `
		SELECT id, project_id, name, visibility_timeout_sec, message_retention_sec, state, created_at, updated_at, status
		FROM queues WHERE project_id = $1 AND name = $2`,
		projectID, name).Scan(
		&q.ID, &q.ProjectID, &q.Name, &q.VisibilityTimeoutSec, &q.MessageRetentionSec, &q.State, &q.CreatedAt, &q.UpdatedAt, &q.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return q, err
}

func (i *Iris) getTopicInProject(ctx context.Context, projectID, name string) (*database.Topic, error) {
	t := &database.Topic{}
	err := i.DB.QueryRow(ctx, `
		SELECT id, project_id, name, state, created_at, updated_at, status
		FROM topics WHERE project_id = $1 AND name = $2`,
		projectID, name).Scan(
		&t.ID, &t.ProjectID, &t.Name, &t.State, &t.CreatedAt, &t.UpdatedAt, &t.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}
