package domain

import "time"

// Kafka topics. Owner service in brackets.
const (
	TopicReceiptUploaded   = "billsplit.receipt.uploaded"   // [split] -> ocr-worker
	TopicReceiptParsed     = "billsplit.receipt.parsed"     // [ocr-worker] -> split
	TopicReceiptFinalized  = "billsplit.receipt.finalized"  // [split] -> finance
	TopicStatementUploaded = "billsplit.statement.uploaded" // [finance] -> finance (async import)
)

var AllTopics = []string{TopicReceiptUploaded, TopicReceiptParsed, TopicReceiptFinalized, TopicStatementUploaded, TopicNotification}

type ReceiptUploadedEvent struct {
	ReceiptID string `json:"receipt_id"`
}

type ParsedItem struct {
	Name       string `json:"name"`               // readable (enriched) name
	RawName    string `json:"raw_name,omitempty"` // as printed on the receipt
	Code       string `json:"code,omitempty"`     // UPC/EAN
	Quantity   int32  `json:"quantity"`
	PriceCents int64  `json:"price_cents"`
}

type ReceiptParsedEvent struct {
	ReceiptID     string       `json:"receipt_id"`
	OK            bool         `json:"ok"`
	Error         string       `json:"error,omitempty"`
	Warning       string       `json:"warning,omitempty"`
	Provider      string       `json:"provider"`
	Store         string       `json:"store"`
	Items         []ParsedItem `json:"items"`
	SubtotalCents int64        `json:"subtotal_cents"`
	TaxCents      int64        `json:"tax_cents"`
	TotalCents    int64        `json:"total_cents"`
	PurchasedAt   *time.Time   `json:"purchased_at,omitempty"`
}

// ReceiptFinalizedEvent carries everything finance needs, so it never calls split.
type ReceiptFinalizedEvent struct {
	ReceiptID  string       `json:"receipt_id"`
	Store      string       `json:"store"`
	Currency   string       `json:"currency"`
	OccurredAt time.Time    `json:"occurred_at"`
	Shares     []ShareEvent `json:"shares"`
}

type ShareEvent struct {
	UserID     string `json:"user_id"`
	TotalCents int64  `json:"total_cents"`
}

type StatementUploadedEvent struct {
	ImportID string `json:"import_id"`
	UserID   string `json:"user_id"`
}

// TopicNotification carries user-facing notifications from any service to the
// notifier (in-app inbox + Expo push). Producers format the text.
const TopicNotification = "billsplit.notification"

// Notification types (used by the client for icons and navigation).
const (
	NotifyFriendRequest     = "friend_request"
	NotifyFriendAccepted    = "friend_accepted"
	NotifyGroupJoined       = "group_joined"
	NotifyReceiptParsed     = "receipt_parsed"
	NotifyReceiptFinalized  = "receipt_finalized"
	NotifyExpenseAdded      = "expense_added"
	NotifySettlement        = "settlement"
	NotifyRecurringUpcoming = "recurring_upcoming"
)

type NotificationEvent struct {
	UserIDs []string          `json:"user_ids"`
	Type    string            `json:"type"`
	Title   string            `json:"title"`
	Body    string            `json:"body"`
	Data    map[string]string `json:"data,omitempty"` // navigation hints: group_id, receipt_id...
}
