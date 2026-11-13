package provider

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestSimulatorRequiresAccountKeyAndPaginates(t *testing.T) {
	s := NewDemo()
	key := t.Name()
	s.AccountKeys = map[string]string{"acct-demo": key}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/charges?account_id=acct-demo&page=1&page_size=100", nil)
	s.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("unauthenticated status = %d", w.Code)
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/v1/charges?account_id=acct-demo&page=1&page_size=100", nil)
	r.Header.Set("Authorization", "Bearer "+key)
	s.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	var response struct {
		Data    []any `json:"data"`
		HasMore bool  `json:"has_more"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data) != 100 || !response.HasMore {
		t.Fatalf("bad first page size=%d has_more=%v", len(response.Data), response.HasMore)
	}
}

func TestSimulatorDetailsAndWindowFiltering(t *testing.T) {
	s := NewDemo()
	key := t.Name()
	s.AccountKeys = map[string]string{"acct-demo": key}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/charges/ch_4", nil)
	r.Header.Set("Authorization", "Bearer "+key)
	s.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("detail status = %d", w.Code)
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/v1/charges?account_id=acct-demo&created_since=2999-01-01T00:00:00Z", nil)
	r.Header.Set("Authorization", "Bearer "+key)
	s.ServeHTTP(w, r)
	var response struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Total != 0 {
		t.Fatalf("future time filter returned %d records", response.Total)
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/v1/charges?account_id=acct-other", nil)
	r.Header.Set("Authorization", "Bearer "+key)
	s.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("cross-account request status = %d", w.Code)
	}
}
