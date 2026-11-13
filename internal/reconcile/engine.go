package reconcile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

func fingerprint(kind, tenant, account, identity string) string {
	s := sha256.Sum256([]byte(strings.Join([]string{kind, tenant, account, identity}, "\x00")))
	return hex.EncodeToString(s[:])[:24]
}

// Evaluate matches only within a tenant's provider account and requires a unique stable reference.
// Incomplete imports suppress missing-record conclusions.
func Evaluate(payments []Payment, snapshot Snapshot) []Finding {
	return EvaluateInput(Input{Payments: payments, Snapshot: snapshot, Now: time.Now().UTC(), GracePeriod: 5 * time.Minute})
}

func EvaluateInput(input Input) []Finding {
	payments, snapshot := input.Payments, input.Snapshot
	byRef := map[string][]Charge{}
	for _, c := range snapshot.Charges {
		byRef[c.AccountID+"\x00"+c.PaymentRef] = append(byRef[c.AccountID+"\x00"+c.PaymentRef], c)
	}
	seen := map[string][]Payment{}
	findings := []Finding{}
	for _, p := range payments {
		owner, configured := snapshot.AccountTenants[p.AccountID]
		if !configured || owner != p.TenantID {
			findings = append(findings, Finding{Fingerprint: fingerprint("account_ownership_unverified", p.TenantID, p.AccountID, p.ID), Type: "account_ownership_unverified", Severity: "medium", Confidence: "uncertain", TenantID: p.TenantID, AccountID: p.AccountID, PaymentID: p.ID, Summary: "Provider account ownership is not verified for this tenant; records were not compared.", Recommendation: "Verify the provider connection to tenant mapping before reconciliation."})
			continue
		}
		key := p.AccountID + "\x00" + p.ProviderPaymentRef
		seen[key] = append(seen[key], p)
		matches := byRef[key]
		if len(matches) > 1 {
			captured := 0
			for _, candidate := range matches {
				if candidate.Status == "succeeded" && candidate.CapturedAt != nil {
					captured++
				}
			}
			kind, summary := "ambiguous_provider_match", "Multiple provider charges share the payment reference; no charge was selected."
			if captured > 1 {
				kind, summary = "potential_duplicate_provider_capture", "Multiple captured provider charges share one payment reference; duplicate capture is possible but not assumed."
			}
			findings = append(findings, Finding{Fingerprint: fingerprint(kind, p.TenantID, p.AccountID, p.ProviderPaymentRef), Type: kind, Severity: "medium", Confidence: "uncertain", TenantID: p.TenantID, AccountID: p.AccountID, PaymentID: p.ID, Summary: summary, Recommendation: "Review provider charge IDs and determine whether the captures are distinct legitimate transactions."})
			continue
		}
		if len(matches) == 0 {
			if snapshot.Complete && p.Status == "succeeded" && !withinGrace(p.CreatedAt, input.Now, input.GracePeriod) {
				findings = append(findings, Finding{Fingerprint: fingerprint("missing_provider_charge", p.TenantID, p.AccountID, p.ID), Type: "missing_provider_charge", Severity: "high", Confidence: "conclusive", TenantID: p.TenantID, AccountID: p.AccountID, PaymentID: p.ID, Currency: p.Currency, ImpactMinor: p.Amount, Summary: "Application records a successful payment with no matching provider charge.", Recommendation: "Inspect provider history and application webhook processing."})
			}
			continue
		}
		c := matches[0]
		if c.AccountID != p.AccountID {
			continue
		}
		if c.Currency != p.Currency {
			findings = append(findings, Finding{Fingerprint: fingerprint("currency_mismatch", p.TenantID, p.AccountID, p.ID), Type: "currency_mismatch", Severity: "high", Confidence: "conclusive", TenantID: p.TenantID, AccountID: p.AccountID, PaymentID: p.ID, ProviderID: c.ID, Currency: p.Currency, Summary: fmt.Sprintf("Currency differs: application %s, provider %s.", p.Currency, c.Currency), Recommendation: "Verify account and transaction mapping."})
			continue
		}
		if p.Status == "succeeded" && c.Status != "succeeded" {
			findings = append(findings, Finding{Fingerprint: fingerprint("unsupported_paid_status", p.TenantID, p.AccountID, p.ID), Type: "unsupported_paid_status", Severity: "high", Confidence: "conclusive", TenantID: p.TenantID, AccountID: p.AccountID, PaymentID: p.ID, ProviderID: c.ID, Currency: p.Currency, ImpactMinor: p.Amount, Summary: "Application records success but provider charge is not captured.", Recommendation: "Check for another successful payment and provider lifecycle updates."})
		}
		if c.Status == "succeeded" && c.CapturedAt != nil && p.Status != "succeeded" && !withinGrace(c.CreatedAt, input.Now, input.GracePeriod) {
			findings = append(findings, Finding{Fingerprint: fingerprint("provider_charge_not_recorded", p.TenantID, p.AccountID, p.ID), Type: "provider_charge_not_recorded", Severity: "high", Confidence: "conclusive", TenantID: p.TenantID, AccountID: p.AccountID, PaymentID: p.ID, ProviderID: c.ID, Currency: c.Currency, ImpactMinor: c.Amount, Summary: "Provider captured funds but application payment is not successful.", Recommendation: "Inspect delayed webhook delivery and invoice payment allocation."})
		}
		if c.Status == "succeeded" && c.Amount != p.Amount {
			if c.Amount < 0 || p.Amount < 0 {
				continue
			}
			delta := c.Amount - p.Amount
			if delta < 0 {
				delta = p.Amount - c.Amount
			}
			findings = append(findings, Finding{Fingerprint: fingerprint("amount_mismatch", p.TenantID, p.AccountID, p.ID), Type: "amount_mismatch", Severity: "high", Confidence: "conclusive", TenantID: p.TenantID, AccountID: p.AccountID, PaymentID: p.ID, ProviderID: c.ID, Currency: c.Currency, ImpactMinor: delta, Summary: fmt.Sprintf("Captured amount %d differs from application amount %d (minor units).", c.Amount, p.Amount), Recommendation: "Verify the expected amount and investigate before adjusting either record."})
		}
	}
	findings = append(findings, evaluateRefunds(input)...)
	findings = append(findings, evaluateSubscriptions(input)...)
	for _, group := range seen {
		if len(group) > 1 {
			ids := make([]string, 0, len(group))
			for _, p := range group {
				ids = append(ids, p.ID)
			}
			sort.Strings(ids)
			p := group[0]
			findings = append(findings, Finding{Fingerprint: fingerprint("duplicate_internal_credit", p.TenantID, p.AccountID, strings.Join(ids, ",")), Type: "duplicate_internal_credit", Severity: "critical", Confidence: "conclusive", TenantID: p.TenantID, AccountID: p.AccountID, PaymentID: strings.Join(ids, ","), ProviderID: p.ProviderPaymentRef, Currency: p.Currency, ImpactMinor: group[0].Amount, Summary: "Multiple internal payment records point to one provider payment reference.", Recommendation: "Review duplicate processing and preserve the original audit trail."})
		}
	}
	if snapshot.Complete {
		for _, c := range snapshot.Charges {
			if c.Status != "succeeded" || c.CapturedAt == nil {
				continue
			}
			matched := false
			owner, configured := snapshot.AccountTenants[c.AccountID]
			if !configured {
				continue
			}
			for _, p := range payments {
				if p.AccountID == c.AccountID && p.ProviderPaymentRef == c.PaymentRef && (!configured || p.TenantID == owner) {
					matched = true
					break
				}
			}
			if !matched {
				tenant := owner
				findings = append(findings, Finding{Fingerprint: fingerprint("unrecognized_provider_capture", tenant, c.AccountID, c.ID), Type: "unrecognized_provider_capture", Severity: "high", Confidence: "conclusive", TenantID: tenant, AccountID: c.AccountID, ProviderID: c.ID, Currency: c.Currency, ImpactMinor: c.Amount, Summary: "Captured provider charge has no matching application payment.", Recommendation: "Identify the customer and determine whether an application record is missing."})
			}
		}
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].Fingerprint < findings[j].Fingerprint })
	return findings
}

func evaluateRefunds(input Input) []Finding {
	if !input.Snapshot.Complete {
		return nil
	}
	byID := map[string][]Refund{}
	for _, r := range input.Snapshot.Refunds {
		byID[r.AccountID+"\x00"+r.ID] = append(byID[r.AccountID+"\x00"+r.ID], r)
	}
	seen := map[string]bool{}
	out := []Finding{}
	for _, r := range input.Refunds {
		if owner, configured := input.Snapshot.AccountTenants[r.AccountID]; !configured || owner != r.TenantID {
			continue
		}
		key := r.AccountID + "\x00" + r.ProviderRefundID
		seen[key] = true
		matches := byID[key]
		if len(matches) == 0 && r.Status == "succeeded" {
			out = append(out, Finding{Fingerprint: fingerprint("refund_missing_provider", r.TenantID, r.AccountID, r.ID), Type: "refund_missing_provider", Severity: "high", Confidence: "conclusive", TenantID: r.TenantID, AccountID: r.AccountID, PaymentID: r.PaymentID, ProviderID: r.ProviderRefundID, Currency: r.Currency, ImpactMinor: r.Amount, Summary: "Application records a successful refund absent from the complete provider snapshot.", Recommendation: "Inspect the provider refund history before changing the application record."})
			continue
		}
		if len(matches) != 1 {
			continue
		}
		p := matches[0]
		if p.Status == "succeeded" && r.Status != "succeeded" {
			out = append(out, Finding{Fingerprint: fingerprint("refund_not_recorded", r.TenantID, r.AccountID, r.ID), Type: "refund_not_recorded", Severity: "high", Confidence: "conclusive", TenantID: r.TenantID, AccountID: r.AccountID, PaymentID: r.PaymentID, ProviderID: p.ID, Currency: p.Currency, ImpactMinor: p.Amount, Summary: "Provider completed a refund that the application does not record as successful.", Recommendation: "Review refund webhook processing and customer balance handling."})
		}
		if p.Amount != r.Amount || p.Currency != r.Currency {
			impact := int64(0)
			if p.Currency == r.Currency {
				if p.Amount > r.Amount {
					impact = p.Amount - r.Amount
				} else {
					impact = r.Amount - p.Amount
				}
			}
			out = append(out, Finding{Fingerprint: fingerprint("refund_amount_mismatch", r.TenantID, r.AccountID, r.ID), Type: "refund_amount_mismatch", Severity: "high", Confidence: "conclusive", TenantID: r.TenantID, AccountID: r.AccountID, PaymentID: r.PaymentID, ProviderID: p.ID, Currency: r.Currency, ImpactMinor: impact, Summary: "Application and provider refund amount or currency differ.", Recommendation: "Compare original refund evidence and avoid treating partial refunds as full refunds."})
		}
	}
	for _, r := range input.Snapshot.Refunds {
		if r.Status == "succeeded" && !seen[r.AccountID+"\x00"+r.ID] {
			out = append(out, Finding{Fingerprint: fingerprint("refund_not_recorded", r.AccountID, r.AccountID, r.ID), Type: "refund_not_recorded", Severity: "high", Confidence: "conclusive", TenantID: "", AccountID: r.AccountID, ProviderID: r.ID, Currency: r.Currency, ImpactMinor: r.Amount, Summary: "Provider completed a refund with no corresponding application refund record.", Recommendation: "Locate the affected payment and inspect refund webhook processing."})
		}
	}
	return out
}

func evaluateSubscriptions(input Input) []Finding {
	if !input.Snapshot.Complete {
		return nil
	}
	providers := map[string][]ProviderSubscription{}
	for _, s := range input.Snapshot.Subscriptions {
		providers[s.AccountID+"\x00"+s.ID] = append(providers[s.AccountID+"\x00"+s.ID], s)
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	grace := input.GracePeriod
	if grace < 0 {
		grace = 0
	}
	out := []Finding{}
	for _, s := range input.Subscriptions {
		if owner, configured := input.Snapshot.AccountTenants[s.AccountID]; !configured || owner != s.TenantID {
			continue
		}
		matches := providers[s.AccountID+"\x00"+s.ProviderSubscriptionID]
		if len(matches) != 1 {
			continue
		}
		p := matches[0]
		if p.Status == s.Status {
			continue
		}
		latest := s.UpdatedAt
		if p.UpdatedAt.After(latest) {
			latest = p.UpdatedAt
		}
		if now.Sub(latest) < grace {
			continue
		}
		out = append(out, Finding{Fingerprint: fingerprint("subscription_status_mismatch", s.TenantID, s.AccountID, s.ID), Type: "subscription_status_mismatch", Severity: "medium", Confidence: "conclusive", TenantID: s.TenantID, AccountID: s.AccountID, PaymentID: s.ID, ProviderID: p.ID, Summary: fmt.Sprintf("Subscription status differs after the %s synchronization grace period: application %s, provider %s.", grace, s.Status, p.Status), Recommendation: "Review trial, grace-period, and cancellation entitlement policy; do not change access automatically."})
	}
	return out
}

func withinGrace(created, now time.Time, grace time.Duration) bool {
	if created.IsZero() || now.IsZero() || grace <= 0 {
		return false
	}
	return now.Sub(created) >= 0 && now.Sub(created) < grace
}
