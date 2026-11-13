package reconcile

import (
	"testing"
	"time"
)

func TestAmountMismatchAndStableFingerprint(t *testing.T) {
	timeCaptured := time.Now().UTC()
	p := Payment{TenantID: "t1", AccountID: "a1", ID: "p1", ProviderPaymentRef: "ref", Amount: 9900, Currency: "USD", Status: "succeeded"}
	s := Snapshot{Complete: true, AccountTenants: map[string]string{"a1": "t1"}, Charges: []Charge{{AccountID: "a1", ID: "ch1", PaymentRef: "ref", Amount: 19900, Currency: "USD", Status: "succeeded", CapturedAt: &timeCaptured}}}
	a, b := Evaluate([]Payment{p}, s), Evaluate([]Payment{p}, s)
	if len(a) != 1 || a[0].Type != "amount_mismatch" || a[0].ImpactMinor != 10000 {
		t.Fatalf("unexpected findings: %+v", a)
	}
	if a[0].Fingerprint != b[0].Fingerprint {
		t.Fatal("fingerprint changed across repeated evaluation")
	}
}
func TestIncompleteSnapshotSuppressesMissingProviderFinding(t *testing.T) {
	p := Payment{TenantID: "t1", AccountID: "a1", ID: "p1", ProviderPaymentRef: "ref", Amount: 1200, Currency: "USD", Status: "succeeded"}
	if got := Evaluate([]Payment{p}, Snapshot{Complete: false, AccountTenants: map[string]string{"a1": "t1"}}); len(got) != 0 {
		t.Fatalf("partial import produced conclusive finding: %+v", got)
	}
}
func TestMatchingIsAccountScoped(t *testing.T) {
	when := time.Now().UTC()
	p := Payment{TenantID: "tenant-A", AccountID: "account-A", ID: "p", ProviderPaymentRef: "same", Amount: 100, Currency: "USD", Status: "succeeded"}
	s := Snapshot{Complete: true, AccountTenants: map[string]string{"account-A": "tenant-A", "account-B": "tenant-B"}, Charges: []Charge{{AccountID: "account-B", ID: "provider", PaymentRef: "same", Amount: 100, Currency: "USD", Status: "succeeded", CapturedAt: &when}}}
	got := Evaluate([]Payment{p}, s)
	for _, finding := range got {
		if finding.Type == "amount_mismatch" || finding.ProviderID == "provider" && finding.PaymentID == "p" {
			t.Fatalf("cross-account records were matched: %+v", got)
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected independent missing and orphan findings, got %+v", got)
	}
}
func TestDuplicateInternalCreditsAreNotSummed(t *testing.T) {
	p := Payment{TenantID: "t", AccountID: "a", ID: "p1", ProviderPaymentRef: "ref", Amount: 500, Currency: "USD", Status: "succeeded"}
	q := p
	q.ID = "p2"
	got := Evaluate([]Payment{p, q}, Snapshot{Complete: true, AccountTenants: map[string]string{"a": "t"}})
	var duplicate *Finding
	for i := range got {
		if got[i].Type == "duplicate_internal_credit" {
			duplicate = &got[i]
		}
	}
	if duplicate == nil || duplicate.ImpactMinor != 500 {
		t.Fatalf("duplicate credit not identified safely: %+v", got)
	}
}

func TestRefundAndSubscriptionRulesRespectGrace(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	input := Input{Now: now, GracePeriod: 5 * time.Minute, Snapshot: Snapshot{Complete: true, AccountTenants: map[string]string{"a": "t"},
		Refunds:       []Refund{{AccountID: "a", ID: "refund-1", ChargeID: "charge-1", Amount: 250, Currency: "USD", Status: "succeeded"}},
		Subscriptions: []ProviderSubscription{{AccountID: "a", ID: "sub-1", Status: "canceled", UpdatedAt: now.Add(-time.Hour)}},
	}, Subscriptions: []ApplicationSubscription{{TenantID: "t", AccountID: "a", ID: "local-sub", ProviderSubscriptionID: "sub-1", Status: "active", UpdatedAt: now.Add(-time.Hour)}}}
	got := EvaluateInput(input)
	types := map[string]bool{}
	for _, f := range got {
		types[f.Type] = true
	}
	if !types["refund_not_recorded"] || !types["subscription_status_mismatch"] {
		t.Fatalf("expected refund and aged subscription findings: %+v", got)
	}
	input.Subscriptions[0].UpdatedAt = now.Add(-time.Minute)
	got = EvaluateInput(input)
	for _, f := range got {
		if f.Type == "subscription_status_mismatch" {
			t.Fatalf("grace period did not defer finding: %+v", got)
		}
	}
	input.Snapshot.Complete = false
	got = EvaluateInput(input)
	for _, f := range got {
		if f.Type == "refund_not_recorded" {
			t.Fatalf("partial snapshot caused missing refund finding: %+v", got)
		}
	}
}

func TestMultipleProviderCapturesRemainUncertain(t *testing.T) {
	tm := time.Now().UTC()
	p := Payment{TenantID: "t", AccountID: "a", ID: "p", ProviderPaymentRef: "ref", Amount: 100, Currency: "USD", Status: "succeeded"}
	s := Snapshot{Complete: true, AccountTenants: map[string]string{"a": "t"}, Charges: []Charge{{AccountID: "a", ID: "c1", PaymentRef: "ref", Amount: 100, Currency: "USD", Status: "succeeded", CapturedAt: &tm}, {AccountID: "a", ID: "c2", PaymentRef: "ref", Amount: 100, Currency: "USD", Status: "succeeded", CapturedAt: &tm}}}
	got := Evaluate([]Payment{p}, s)
	if len(got) != 1 || got[0].Type != "potential_duplicate_provider_capture" || got[0].Confidence != "uncertain" {
		t.Fatalf("duplicate capture asserted without uncertainty: %+v", got)
	}
}

func TestRecentMissingPaymentWaitsForGraceAndAccountOwnershipIsEnforced(t *testing.T) {
	now := time.Now().UTC()
	p := Payment{TenantID: "tenant-wrong", AccountID: "acct", ID: "p", ProviderPaymentRef: "missing", Amount: 100, Currency: "USD", Status: "succeeded", CreatedAt: now.Add(-time.Minute)}
	input := Input{Payments: []Payment{p}, Snapshot: Snapshot{Complete: true, AccountTenants: map[string]string{"acct": "tenant-correct"}}, Now: now, GracePeriod: 5 * time.Minute}
	if got := EvaluateInput(input); len(got) != 1 || got[0].Type != "account_ownership_unverified" {
		t.Fatalf("account mapped to a different tenant was evaluated instead of quarantined: %+v", got)
	}
	p.TenantID = "tenant-correct"
	input.Payments = []Payment{p}
	if got := EvaluateInput(input); len(got) != 0 {
		t.Fatalf("recent payment escaped grace: %+v", got)
	}
	p.CreatedAt = now.Add(-time.Hour)
	input.Payments = []Payment{p}
	if got := EvaluateInput(input); len(got) != 1 || got[0].Type != "missing_provider_charge" {
		t.Fatalf("aged missing payment not detected: %+v", got)
	}
}
