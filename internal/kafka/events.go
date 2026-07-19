// Package kafka wires the riskguard engine into an event-driven pipeline:
// transactions arrive on an input topic, get evaluated, and the resulting
// decision is published to an output topic for downstream consumers
// (ledger, notification service, case management, etc).
package kafka

import (
	"time"

	"github.com/peymanahmadi/riskguard/pkg/riskguard"
)

// TransactionEvent is the wire format consumed from the transactions topic.
type TransactionEvent struct {
	ID            string            `json:"id"`
	EntityID      string            `json:"entity_id"`
	MerchantID    string            `json:"merchant_id"`
	AmountMinor   int64             `json:"amount_minor"`
	Currency      string            `json:"currency"`
	IP            string            `json:"ip"`
	DeviceID      string            `json:"device_id"`
	Country       string            `json:"country"`
	Lat           float64           `json:"lat"`
	Lon           float64           `json:"lon"`
	PaymentMethod string            `json:"payment_method"`
	CreatedAt     time.Time         `json:"created_at"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

func (e TransactionEvent) toTransaction() riskguard.Transaction {
	return riskguard.Transaction{
		ID:            e.ID,
		EntityID:      e.EntityID,
		MerchantID:    e.MerchantID,
		AmountMinor:   e.AmountMinor,
		Currency:      e.Currency,
		IP:            e.IP,
		DeviceID:      e.DeviceID,
		Country:       e.Country,
		Lat:           e.Lat,
		Lon:           e.Lon,
		PaymentMethod: e.PaymentMethod,
		CreatedAt:     e.CreatedAt,
		Metadata:      e.Metadata,
	}
}

// DecisionEvent is the wire format published to the decisions topic after
// evaluation.
type DecisionEvent struct {
	TransactionID string    `json:"transaction_id"`
	EntityID      string    `json:"entity_id"`
	Score         float64   `json:"score"`
	Decision      string    `json:"decision"`
	Reasons       []string  `json:"reasons,omitempty"`
	EvaluatedAt   time.Time `json:"evaluated_at"`
}

func newDecisionEvent(tx riskguard.Transaction, v riskguard.Verdict) DecisionEvent {
	return DecisionEvent{
		TransactionID: v.TransactionID,
		EntityID:      tx.EntityID,
		Score:         v.Score,
		Decision:      v.Decision.String(),
		Reasons:       v.Reasons(),
		EvaluatedAt:   time.Now().UTC(),
	}
}
