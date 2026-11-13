package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ranajahanzaib/payment-reconciliation/internal/reconcile"
)

type Simulator struct {
	Charges       []reconcile.Charge
	Refunds       []reconcile.Refund
	Subscriptions []reconcile.ProviderSubscription
	APIKey        string
	AccountKeys   map[string]string
	FailAfterPage int
}

func (s *Simulator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
		return
	}
	account := r.URL.Query().Get("account_id")
	if account == "" {
		account = s.recordAccount(r.URL.Path)
	}
	expected := s.APIKey
	if key, ok := s.AccountKeys[account]; ok {
		expected = key
	} else if len(s.AccountKeys) > 0 {
		http.Error(w, "unknown provider account", http.StatusUnauthorized)
		return
	}
	if expected == "" || r.Header.Get("Authorization") != "Bearer "+expected {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/charges/") {
		id := strings.TrimPrefix(r.URL.Path, "/v1/charges/")
		for _, v := range s.Charges {
			if v.ID == id {
				writeJSON(w, v)
				return
			}
		}
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/refunds/") {
		id := strings.TrimPrefix(r.URL.Path, "/v1/refunds/")
		for _, v := range s.Refunds {
			if v.ID == id {
				writeJSON(w, v)
				return
			}
		}
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/subscriptions/") {
		id := strings.TrimPrefix(r.URL.Path, "/v1/subscriptions/")
		for _, v := range s.Subscriptions {
			if v.ID == id {
				writeJSON(w, v)
				return
			}
		}
		http.NotFound(w, r)
		return
	}
	if r.URL.Path != "/v1/charges" && r.URL.Path != "/v1/refunds" && r.URL.Path != "/v1/subscriptions" {
		http.NotFound(w, r)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if size < 1 || size > 500 {
		size = 100
	}
	if s.FailAfterPage > 0 && page > s.FailAfterPage {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "simulated pagination interruption", http.StatusServiceUnavailable)
		return
	}
	var rows any
	total := 0
	if r.URL.Path == "/v1/charges" {
		filtered := make([]reconcile.Charge, 0)
		account := r.URL.Query().Get("account_id")
		for _, v := range s.Charges {
			if account == "" || v.AccountID == account {
				if !inWindow(v.CreatedAt, v.UpdatedAt, r) {
					continue
				}
				filtered = append(filtered, v)
			}
		}
		total = len(filtered)
		rows = slice(filtered, page, size)
	} else if r.URL.Path == "/v1/refunds" {
		filtered := make([]reconcile.Refund, 0)
		account := r.URL.Query().Get("account_id")
		for _, v := range s.Refunds {
			if account == "" || v.AccountID == account {
				if !inWindow(v.CreatedAt, v.UpdatedAt, r) {
					continue
				}
				filtered = append(filtered, v)
			}
		}
		total = len(filtered)
		rows = slice(filtered, page, size)
	} else {
		filtered := make([]reconcile.ProviderSubscription, 0)
		account := r.URL.Query().Get("account_id")
		for _, v := range s.Subscriptions {
			if account == "" || v.AccountID == account {
				if !inWindow(v.UpdatedAt, v.UpdatedAt, r) {
					continue
				}
				filtered = append(filtered, v)
			}
		}
		total = len(filtered)
		rows = slice(filtered, page, size)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": rows, "page": page, "page_size": size, "total": total, "has_more": page*size < total})
}
func slice[T any](v []T, p, n int) []T {
	start := (p - 1) * n
	if start >= len(v) {
		return []T{}
	}
	end := start + n
	if end > len(v) {
		end = len(v)
	}
	return v[start:end]
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Simulator) recordAccount(path string) string {
	id := path[strings.LastIndex(path, "/")+1:]
	for _, v := range s.Charges {
		if v.ID == id {
			return v.AccountID
		}
	}
	for _, v := range s.Refunds {
		if v.ID == id {
			return v.AccountID
		}
	}
	for _, v := range s.Subscriptions {
		if v.ID == id {
			return v.AccountID
		}
	}
	return ""
}
func inWindow(created, updated time.Time, r *http.Request) bool {
	for key, value := range map[string]time.Time{"created_since": created, "updated_since": updated} {
		raw := r.URL.Query().Get(key)
		if raw == "" {
			continue
		}
		cutoff, err := time.Parse(time.RFC3339, raw)
		if err != nil || value.IsZero() || value.Before(cutoff) {
			return false
		}
	}
	return true
}
func NewDemo() *Simulator {
	now := time.Now().UTC()
	s := &Simulator{}
	for i := 0; i < 1000; i++ {
		ref := "pay_" + strconv.Itoa(i)
		status := "succeeded"
		amount := int64(9900)
		if i == 1 {
			status = "failed"
		}
		if i == 2 {
			amount = 19900
		}
		if i == 3 {
			continue
		}
		t := now.Add(-time.Hour)
		var captured *time.Time
		if status == "succeeded" {
			captured = &t
		}
		s.Charges = append(s.Charges, reconcile.Charge{AccountID: "acct-demo", ID: "ch_" + strconv.Itoa(i), PaymentRef: ref, InvoiceRef: "inv_" + strconv.Itoa(i), Amount: amount, Currency: "USD", Status: status, CreatedAt: t, UpdatedAt: t, CapturedAt: captured})
	}
	s.Refunds = append(s.Refunds, reconcile.Refund{AccountID: "acct-demo", ID: "re_demo", ChargeID: "ch_4", Amount: 2500, Currency: "USD", Status: "succeeded", CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour)})
	// One captured charge has no corresponding application record.
	t := now.Add(-time.Hour)
	s.Charges = append(s.Charges, reconcile.Charge{AccountID: "acct-demo", ID: "ch_orphan", PaymentRef: "pay_orphan", InvoiceRef: "inv_orphan", Amount: 9900, Currency: "USD", Status: "succeeded", CreatedAt: t, UpdatedAt: t, CapturedAt: &t})
	s.Subscriptions = append(s.Subscriptions, reconcile.ProviderSubscription{AccountID: "acct-demo", ID: "sub_demo", Status: "canceled", UpdatedAt: t})
	return s
}
func FetchAll(client *http.Client, base, key, resource, account string, pageSize int) ([]json.RawMessage, bool, error) {
	return FetchAllContext(context.Background(), client, base, key, resource, account, pageSize, 3)
}
func FetchAllContext(ctx context.Context, client *http.Client, base, key, resource, account string, pageSize, maxRetries int) ([]json.RawMessage, bool, error) {
	items := []json.RawMessage{}
	complete := true
	if pageSize < 1 || pageSize > 500 {
		return items, false, fmt.Errorf("page size must be between 1 and 500")
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	if maxRetries > 5 {
		maxRetries = 5
	}
	for page := 1; ; page++ {
		url := strings.TrimRight(base, "/") + resource + "?account_id=" + account + "&page=" + strconv.Itoa(page) + "&page_size=" + strconv.Itoa(pageSize)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		req.Header.Set("Authorization", "Bearer "+key)
		resp, err := doRetry(client, req, maxRetries)
		if err != nil {
			return items, false, fmt.Errorf("fetch %s page %d: %w", resource, page, err)
		}
		var envelope struct {
			Data    []json.RawMessage `json:"data"`
			HasMore bool              `json:"has_more"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&envelope)
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 {
			return items, false, fmt.Errorf("fetch %s page %d: %w", resource, page, errOrStatus(err, resp.StatusCode))
		}
		items = append(items, envelope.Data...)
		if !envelope.HasMore {
			break
		}
	}
	return items, complete, nil
}
func doRetry(client *http.Client, req *http.Request, maxRetries int) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := client.Do(req.Clone(req.Context()))
		if err == nil && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) {
			lastErr = errOrStatus(nil, resp.StatusCode)
			delay := retryDelay(resp.Header.Get("Retry-After"), attempt)
			resp.Body.Close()
			if attempt == maxRetries {
				return nil, lastErr
			}
			timer := time.NewTimer(delay)
			select {
			case <-req.Context().Done():
				timer.Stop()
				return nil, req.Context().Err()
			case <-timer.C:
			}
			continue
		}
		if err != nil {
			lastErr = err
			if attempt == maxRetries {
				return nil, lastErr
			}
			timer := time.NewTimer(time.Duration(50*(1<<attempt)) * time.Millisecond)
			select {
			case <-req.Context().Done():
				timer.Stop()
				return nil, req.Context().Err()
			case <-timer.C:
			}
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}
func retryDelay(value string, attempt int) time.Duration {
	delay := time.Duration(50*(1<<attempt)) * time.Millisecond
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 && time.Duration(seconds)*time.Second > delay {
		delay = time.Duration(seconds) * time.Second
	} else if date, err := http.ParseTime(value); err == nil && time.Until(date) > delay {
		delay = time.Until(date)
	}
	if delay > 2*time.Second {
		delay = 2 * time.Second
	}
	return delay
}
func errOrStatus(err error, status int) error {
	if err != nil {
		return err
	}
	return &statusError{status}
}

type statusError struct{ status int }

func (e *statusError) Error() string { return "provider returned HTTP " + strconv.Itoa(e.status) }
