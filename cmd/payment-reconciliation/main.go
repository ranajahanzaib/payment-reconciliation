package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ranajahanzaib/payment-reconciliation/internal/provider"
	"github.com/ranajahanzaib/payment-reconciliation/internal/reconcile"
)

type report struct {
	SchemaVersion      string              `json:"schema_version"`
	GeneratedAt        time.Time           `json:"generated_at"`
	Snapshot           reconcile.Snapshot  `json:"coverage"`
	ProviderRecords    int                 `json:"provider_records"`
	ApplicationRecords int                 `json:"application_records"`
	Findings           []reconcile.Finding `json:"findings"`
	Verification       verification        `json:"verification"`
}
type verification struct {
	Expected   map[string]int `json:"expected"`
	Detected   map[string]int `json:"detected"`
	Missed     map[string]int `json:"missed"`
	Unexpected map[string]int `json:"unexpected"`
	Passed     bool           `json:"passed"`
}
type reviewEvent struct {
	Fingerprint string    `json:"fingerprint"`
	Status      string    `json:"status"`
	Note        string    `json:"note"`
	Actor       string    `json:"actor"`
	At          time.Time `json:"at"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "provider-sim" {
		runProvider()
		return
	}
	cmd := "demo"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "demo":
		runDemo()
	case "serve":
		runServe()
	default:
		fmt.Fprintln(os.Stderr, "usage: payment-reconciliation [demo|serve|provider-sim]")
		os.Exit(2)
	}
}
func runProvider() {
	addr := env("PROVIDER_ADDR", ":8081")
	sim := provider.NewDemo()
	key := os.Getenv("PROVIDER_API_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "PROVIDER_API_KEY is required; generate one with: export PROVIDER_API_KEY=\"$(openssl rand -hex 32)\"")
		os.Exit(2)
	}
	sim.AccountKeys = map[string]string{"acct-demo": key}
	fmt.Println("provider simulator listening on", addr)
	if err := http.ListenAndServe(addr, sim); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func seedPayments() []reconcile.Payment {
	now := time.Now().UTC()
	out := make([]reconcile.Payment, 0, 1001)
	for i := 0; i < 1000; i++ {
		amount := int64(9900)
		status := "succeeded"
		if i == 2 {
			amount = 9900
		} // provider captures 199.00
		out = append(out, reconcile.Payment{TenantID: "tenant-demo", AccountID: "acct-demo", ID: fmt.Sprintf("app_%04d", i), InvoiceID: fmt.Sprintf("inv_%04d", i), ProviderPaymentRef: fmt.Sprintf("pay_%d", i), Amount: amount, Currency: "USD", Status: status, CreatedAt: now.Add(-2 * time.Hour)})
	}
	// Two application credits reference the one payment represented by pay_4.
	extra := out[4]
	extra.ID = "app_duplicate_4"
	out = append(out, extra)
	// A second tenant reuses the textual reference but is bound to another account.
	out = append(out, reconcile.Payment{TenantID: "tenant-b", AccountID: "acct-b", ID: "app_overlap", ProviderPaymentRef: "pay_0", Amount: 9900, Currency: "USD", Status: "succeeded", CreatedAt: now})
	return out
}
func collect(base, key string) (reconcile.Snapshot, error) {
	timeout, err := time.ParseDuration(env("PROVIDER_REQUEST_TIMEOUT", "5s"))
	if err != nil || timeout <= 0 {
		return reconcile.Snapshot{}, fmt.Errorf("invalid PROVIDER_REQUEST_TIMEOUT")
	}
	pageSize, err := strconv.Atoi(env("PROVIDER_PAGE_SIZE", "100"))
	if err != nil || pageSize < 1 || pageSize > 500 {
		return reconcile.Snapshot{}, fmt.Errorf("PROVIDER_PAGE_SIZE must be between 1 and 500")
	}
	retries, err := strconv.Atoi(env("PROVIDER_MAX_RETRIES", "3"))
	if err != nil || retries < 0 || retries > 5 {
		return reconcile.Snapshot{}, fmt.Errorf("PROVIDER_MAX_RETRIES must be between 0 and 5")
	}
	c := &http.Client{Timeout: timeout}
	fetch := func(resource string) ([]json.RawMessage, bool, error) {
		return provider.FetchAllContext(context.Background(), c, base, key, resource, "acct-demo", pageSize, retries)
	}
	raw, ok, err := fetch("/v1/charges")
	snap := reconcile.Snapshot{Complete: ok}
	if err != nil {
		snap.Errors = append(snap.Errors, err.Error())
		return snap, err
	}
	snap.Charges = make([]reconcile.Charge, 0, len(raw))
	for _, b := range raw {
		var x reconcile.Charge
		if err = json.Unmarshal(b, &x); err != nil {
			snap.Complete = false
			snap.Errors = append(snap.Errors, "invalid charge payload")
			continue
		}
		snap.Charges = append(snap.Charges, x)
	}
	raws, ok, err := fetch("/v1/refunds")
	snap.Complete = snap.Complete && ok
	if err != nil {
		snap.Errors = append(snap.Errors, err.Error())
		return snap, err
	}
	snap.Refunds = make([]reconcile.Refund, 0, len(raws))
	for _, b := range raws {
		var x reconcile.Refund
		if json.Unmarshal(b, &x) == nil {
			snap.Refunds = append(snap.Refunds, x)
		} else {
			snap.Complete = false
		}
	}
	raws, ok, err = fetch("/v1/subscriptions")
	snap.Complete = snap.Complete && ok
	if err != nil {
		snap.Errors = append(snap.Errors, err.Error())
		return snap, err
	}
	for _, b := range raws {
		var x reconcile.ProviderSubscription
		if json.Unmarshal(b, &x) != nil {
			snap.Complete = false
			snap.Errors = append(snap.Errors, "invalid subscription payload")
			continue
		}
		snap.Subscriptions = append(snap.Subscriptions, x)
	}
	return snap, nil
}
func runDemo() {
	api := flag.NewFlagSet("demo", flag.ExitOnError)
	outdir := api.String("out", "artifacts/demo", "report output directory")
	_ = api.Parse(os.Args[2:])
	sim := provider.NewDemo()
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		fatal(err)
	}
	key := hex.EncodeToString(keyBytes)
	sim.AccountKeys = map[string]string{"acct-demo": key}
	srv := &http.Server{Addr: "127.0.0.1:0", Handler: sim}
	ln, err := listen(srv)
	var snap reconcile.Snapshot
	adapterMode := "HTTP simulator adapter"
	if err == nil {
		go func() { _ = srv.Serve(ln) }()
		defer srv.Shutdown(context.Background())
		snap, err = collect("http://"+ln.Addr().String(), key)
		if err != nil {
			fatal(err)
		}
	} else {
		adapterMode = "offline fixture adapter (local sockets unavailable)"
		// Round-trip through provider JSON wire records to exercise normalization without TCP.
		snap = reconcile.Snapshot{Complete: true}
		for _, c := range sim.Charges {
			b, _ := json.Marshal(c)
			var normalized reconcile.Charge
			if json.Unmarshal(b, &normalized) != nil {
				snap.Complete = false
				continue
			}
			snap.Charges = append(snap.Charges, normalized)
		}
		snap.Refunds = append(snap.Refunds, sim.Refunds...)
		snap.Subscriptions = append(snap.Subscriptions, sim.Subscriptions...)
	}
	payments := seedPayments()
	snap.AccountTenants = map[string]string{"acct-demo": "tenant-demo", "acct-b": "tenant-b"}
	now := time.Now().UTC()
	grace, err := time.ParseDuration(env("RECONCILIATION_GRACE_PERIOD", "30m"))
	if err != nil || grace < 0 {
		fatal(fmt.Errorf("invalid RECONCILIATION_GRACE_PERIOD"))
	}
	input := reconcile.Input{Payments: payments, Snapshot: snap, Now: now, GracePeriod: grace, Subscriptions: []reconcile.ApplicationSubscription{{TenantID: "tenant-demo", AccountID: "acct-demo", ID: "app_sub_demo", ProviderSubscriptionID: "sub_demo", Status: "active", UpdatedAt: now.Add(-time.Hour)}}}
	findings := reconcile.EvaluateInput(input)
	// Repeat evaluation to assert deterministic, idempotent finding identities.
	repeat := reconcile.EvaluateInput(input)
	if len(findings) != len(repeat) {
		fatal(fmt.Errorf("repeat reconciliation changed result count"))
	}
	for i := range findings {
		if findings[i].Fingerprint != repeat[i].Fingerprint {
			fatal(fmt.Errorf("non-deterministic finding identity"))
		}
	}
	expectedBytes, e := os.ReadFile("fixtures/demo/expected.json")
	if e != nil {
		fatal(e)
	}
	var expected map[string]int
	if e = json.Unmarshal(expectedBytes, &expected); e != nil {
		fatal(e)
	}
	detected := map[string]int{}
	for _, f := range findings {
		detected[f.Type]++
	}
	verificationResult := verification{Expected: expected, Detected: detected, Missed: map[string]int{}, Unexpected: map[string]int{}, Passed: true}
	for kind, count := range expected {
		if detected[kind] < count {
			verificationResult.Missed[kind] = count - detected[kind]
			verificationResult.Passed = false
		}
	}
	for kind, count := range detected {
		if count > expected[kind] {
			verificationResult.Unexpected[kind] = count - expected[kind]
			verificationResult.Passed = false
		}
	}
	if !verificationResult.Passed {
		fatal(fmt.Errorf("demo fixture verification failed: expected %v, detected %v", expected, detected))
	}
	r := report{SchemaVersion: "payment-reconciliation/v1", GeneratedAt: time.Now().UTC(), Snapshot: snap, ProviderRecords: len(snap.Charges), ApplicationRecords: len(payments), Findings: findings, Verification: verificationResult}
	if err = os.MkdirAll(*outdir, 0755); err != nil {
		fatal(err)
	}
	if err = writeJSON(filepath.Join(*outdir, "report.json"), r); err != nil {
		fatal(err)
	}
	if err = writeCSV(filepath.Join(*outdir, "report.csv"), findings); err != nil {
		fatal(err)
	}
	if err = writeHTML(filepath.Join(*outdir, "report.html"), r); err != nil {
		fatal(err)
	}
	counts := map[string]int{}
	for _, f := range findings {
		counts[f.Type]++
	}
	fmt.Printf("Payment Reconciliation\nSnapshot: %s\nProvider charge attempts: %d\nApplication records: %d\nFindings: %d\n", completeLabel(snap.Complete), len(snap.Charges), len(payments), len(findings))
	kinds := make([]string, 0, len(counts))
	for k := range counts {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Printf("  %-32s %d\n", k, counts[k])
	}
	if err = writeJSON(filepath.Join(*outdir, "verification.json"), verificationResult); err != nil {
		fatal(err)
	}
	fmt.Printf("Adapter: %s\nFixture verification: %t (expected %d categories; detected %d)\nRepeat run: stable (%d fingerprints)\nReports: %s\n", adapterMode, verificationResult.Passed, len(expected), len(detected), len(findings), *outdir)
}
func listen(s *http.Server) (net.Listener, error) { return net.Listen("tcp", s.Addr) }
func runServe() {
	var current report
	if data, err := os.ReadFile(env("REPORT_PATH", "artifacts/demo/report.json")); err == nil {
		if err = json.Unmarshal(data, &current); err != nil {
			fatal(err)
		}
	}
	reviewPath := env("REVIEW_LOG_PATH", "artifacts/demo/reviews.jsonl")
	reviews := map[string]reviewEvent{}
	if file, e := os.Open(reviewPath); e == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			var event reviewEvent
			if json.Unmarshal(scanner.Bytes(), &event) == nil {
				reviews[event.Fingerprint] = event
			}
		}
	}
	var reviewMu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"status":"ok"}`)) })
	mux.HandleFunc("/api/summary", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"finding_count": len(current.Findings), "complete": current.Snapshot.Complete, "provider_records": current.ProviderRecords, "application_records": current.ApplicationRecords})
	})
	mux.HandleFunc("/api/findings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		rows := make([]reconcile.Finding, 0)
		for _, f := range current.Findings {
			if (r.URL.Query().Get("type") == "" || r.URL.Query().Get("type") == f.Type) && (r.URL.Query().Get("severity") == "" || r.URL.Query().Get("severity") == f.Severity) {
				rows = append(rows, f)
			}
		}
		json.NewEncoder(w).Encode(rows)
	})
	mux.HandleFunc("/api/findings/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/findings/")
		parts := strings.Split(path, "/")
		finger := parts[0]
		var found *reconcile.Finding
		for i := range current.Findings {
			if current.Findings[i].Fingerprint == finger {
				found = &current.Findings[i]
				break
			}
		}
		if found == nil {
			http.NotFound(w, r)
			return
		}
		if len(parts) == 1 && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			reviewMu.Lock()
			defer reviewMu.Unlock()
			json.NewEncoder(w).Encode(map[string]any{"finding": found, "review": reviews[finger]})
			return
		}
		if len(parts) != 2 || parts[1] != "review" || r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Status string `json:"status"`
			Note   string `json:"note"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&body); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if body.Status != "acknowledged" && body.Status != "investigating" && body.Status != "resolved" {
			http.Error(w, "status must be acknowledged, investigating, or resolved", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(body.Note) == "" {
			http.Error(w, "note is required", http.StatusBadRequest)
			return
		}
		event := reviewEvent{Fingerprint: finger, Status: body.Status, Note: body.Note, Actor: env("REVIEW_ACTOR", "local-user"), At: time.Now().UTC()}
		reviewMu.Lock()
		defer reviewMu.Unlock()
		if err := os.MkdirAll(filepath.Dir(reviewPath), 0755); err != nil {
			http.Error(w, "cannot store review audit", http.StatusInternalServerError)
			return
		}
		f, err := os.OpenFile(reviewPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			http.Error(w, "cannot store review audit", http.StatusInternalServerError)
			return
		}
		err = json.NewEncoder(f).Encode(event)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			http.Error(w, "cannot store review audit", http.StatusInternalServerError)
			return
		}
		reviews[finger] = event
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(event)
	})
	mux.HandleFunc("/api/runs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{{"id": "latest", "generated_at": current.GeneratedAt, "complete": current.Snapshot.Complete, "findings": len(current.Findings)}})
	})
	mux.HandleFunc("/api/reports/latest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(current)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<!doctype html><meta charset='utf-8'><title>Payment Reconciliation</title><style>body{font:16px system-ui;max-width:1100px;margin:3rem auto;color:#18212f}table{border-collapse:collapse;width:100%%}td,th{padding:.65rem;border-bottom:1px solid #ddd;text-align:left}</style><h1>Payment Reconciliation</h1><p>Coverage: <b>%s</b> · Provider records: %d · Application records: %d · Findings: %d</p><p><a href='/?severity=high'>High severity</a> · <a href='/'>All findings</a> · <a href='/api/reports/latest.json'>JSON report</a></p><table><tr><th>Type</th><th>Severity</th><th>Confidence</th><th>Tenant</th><th>Evidence</th><th>Impact</th></tr>", completeLabel(current.Snapshot.Complete), current.ProviderRecords, current.ApplicationRecords, len(current.Findings))
		for _, f := range current.Findings {
			if r.URL.Query().Get("severity") != "" && r.URL.Query().Get("severity") != f.Severity {
				continue
			}
			fmt.Fprintf(w, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%d %s</td></tr>", template.HTMLEscapeString(f.Type), template.HTMLEscapeString(f.Severity), template.HTMLEscapeString(f.Confidence), template.HTMLEscapeString(f.TenantID), template.HTMLEscapeString(f.Summary), f.ImpactMinor, template.HTMLEscapeString(f.Currency))
		}
		fmt.Fprint(w, "</table><p>Read-only report view. Financial records are never changed.</p>")
	})
	fmt.Println("dashboard listening on", env("HTTP_ADDR", "127.0.0.1:8080"))
	fatal(http.ListenAndServe(env("HTTP_ADDR", "127.0.0.1:8080"), mux))
}
func writeJSON(path string, r any) error {
	f, e := os.Create(path)
	if e != nil {
		return e
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
func writeCSV(path string, findings []reconcile.Finding) error {
	f, e := os.Create(path)
	if e != nil {
		return e
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"fingerprint", "type", "severity", "confidence", "tenant_id", "account_id", "payment_id", "provider_id", "currency", "impact_minor", "summary"})
	for _, v := range findings {
		_ = w.Write([]string{v.Fingerprint, v.Type, v.Severity, v.Confidence, v.TenantID, v.AccountID, v.PaymentID, v.ProviderID, v.Currency, fmt.Sprint(v.ImpactMinor), v.Summary})
	}
	w.Flush()
	return w.Error()
}
func writeHTML(path string, r report) error {
	f, e := os.Create(path)
	if e != nil {
		return e
	}
	defer f.Close()
	const tpl = `<!doctype html><meta charset="utf-8"><title>Reconciliation report</title><h1>Payment Reconciliation</h1><p>Coverage: {{if .Snapshot.Complete}}complete{{else}}incomplete{{end}} · Provider charges: {{.ProviderRecords}} · Findings: {{len .Findings}}</p><table border="1" cellpadding="6"><tr><th>Type</th><th>Severity</th><th>Tenant</th><th>Summary</th><th>Impact (minor)</th></tr>{{range .Findings}}<tr><td>{{.Type}}</td><td>{{.Severity}}</td><td>{{.TenantID}}</td><td>{{.Summary}}</td><td>{{.ImpactMinor}}</td></tr>{{end}}</table>`
	return template.Must(template.New("r").Parse(tpl)).Execute(f, r)
}
func completeLabel(v bool) string {
	if v {
		return "COMPLETE"
	}
	return "INCOMPLETE"
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
