package reconcile

import "time"

type Charge struct {
	AccountID  string     `json:"account_id"`
	ID         string     `json:"id"`
	PaymentRef string     `json:"payment_reference"`
	InvoiceRef string     `json:"invoice_reference,omitempty"`
	Amount     int64      `json:"amount_minor"`
	Currency   string     `json:"currency"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	CapturedAt *time.Time `json:"captured_at,omitempty"`
}
type Payment struct {
	TenantID           string    `json:"tenant_id"`
	AccountID          string    `json:"account_id"`
	ID                 string    `json:"id"`
	InvoiceID          string    `json:"invoice_id"`
	ProviderPaymentRef string    `json:"provider_payment_reference"`
	Amount             int64     `json:"amount_minor"`
	Currency           string    `json:"currency"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
}
type Refund struct {
	AccountID string    `json:"account_id"`
	ID        string    `json:"id"`
	ChargeID  string    `json:"charge_id"`
	Amount    int64     `json:"amount_minor"`
	Currency  string    `json:"currency"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type ApplicationRefund struct {
	TenantID         string    `json:"tenant_id"`
	AccountID        string    `json:"account_id"`
	ID               string    `json:"id"`
	PaymentID        string    `json:"payment_id"`
	ProviderRefundID string    `json:"provider_refund_id"`
	Amount           int64     `json:"amount_minor"`
	Currency         string    `json:"currency"`
	Status           string    `json:"status"`
	UpdatedAt        time.Time `json:"updated_at"`
}
type ProviderSubscription struct {
	AccountID string    `json:"account_id"`
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}
type ApplicationSubscription struct {
	TenantID               string    `json:"tenant_id"`
	AccountID              string    `json:"account_id"`
	ID                     string    `json:"id"`
	ProviderSubscriptionID string    `json:"provider_subscription_id"`
	Status                 string    `json:"status"`
	UpdatedAt              time.Time `json:"updated_at"`
}
type Input struct {
	Payments      []Payment
	Refunds       []ApplicationRefund
	Subscriptions []ApplicationSubscription
	Snapshot      Snapshot
	Now           time.Time
	GracePeriod   time.Duration
}
type Finding struct {
	Fingerprint    string `json:"fingerprint"`
	Type           string `json:"type"`
	Severity       string `json:"severity"`
	Confidence     string `json:"confidence"`
	TenantID       string `json:"tenant_id"`
	AccountID      string `json:"account_id"`
	PaymentID      string `json:"payment_id,omitempty"`
	ProviderID     string `json:"provider_id,omitempty"`
	Currency       string `json:"currency,omitempty"`
	ImpactMinor    int64  `json:"impact_minor"`
	Summary        string `json:"summary"`
	Recommendation string `json:"recommendation"`
}

type Snapshot struct {
	Charges        []Charge               `json:"charges"`
	Refunds        []Refund               `json:"refunds"`
	Subscriptions  []ProviderSubscription `json:"subscriptions"`
	AccountTenants map[string]string      `json:"account_tenants,omitempty"`
	Complete       bool                   `json:"complete"`
	Errors         []string               `json:"errors,omitempty"`
}
