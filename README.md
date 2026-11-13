# Payment Reconciliation

Find payment records that disagree between an application and its payment provider. For example, the provider may report a captured $99 payment while the application has no successful payment record. This project compares typed application records with a separately served provider simulator and produces evidence-backed findings. It never changes financial data.

## What it does

The repository contains a Go reconciliation core, deterministic 1,000-charge provider simulator, account-scoped paginated HTTP API, HTTP collection adapter, offline demo path, and JSON/CSV/HTML reports. Money is stored in integer minor units. Stable provider references are only compared inside their configured account. Incomplete imports suppress definitive “missing” findings.

This is a local prototype. PostgreSQL persistence and migrations, historical run storage, database-backed integration tests, and a production-grade multi-user security model are outside its current scope. Review actions are recorded in a local JSONL audit log. Refund and subscription mismatch rules are implemented in the domain evaluator.

## Quick start

Requires Go 1.23 or newer. No external credentials or services are needed for the local demo.

```sh
make test
make demo
```

The demo writes `artifacts/demo/report.json`, `report.csv`, `report.html`, and `verification.json`. It generates exactly 1,000 provider charge attempts, executes the matching engine, checks output against `fixtures/demo/expected.json`, and runs the same evaluation twice to check stable finding fingerprints. If local TCP sockets are available it collects through the HTTP simulator; otherwise it uses the offline fixture adapter and prints which adapter ran.

Start the provider simulator independently:

```sh
export PROVIDER_API_KEY="$(openssl rand -hex 32)"
make provider-sim
```

The health endpoint is `http://127.0.0.1:8081/healthz`. To fetch demo charges, send the account and bearer token: `curl -H "Authorization: Bearer $PROVIDER_API_KEY" 'http://127.0.0.1:8081/v1/charges?account_id=acct-demo'`. The simulator refuses to start without `PROVIDER_API_KEY`.

The simulator supports collection and individual-record `GET /v1/charges`, `/v1/refunds`, and `/v1/subscriptions` routes, `account_id`, RFC3339 `created_since`/`updated_since`, `page`, and `page_size` query parameters (maximum 500), deterministic ordering, and account-scoped bearer-key authentication. `PROVIDER_API_KEY` and `PROVIDER_ADDR` configure it. The demo also reads `PROVIDER_PAGE_SIZE`, `PROVIDER_REQUEST_TIMEOUT`, `PROVIDER_MAX_RETRIES`, and `RECONCILIATION_GRACE_PERIOD`.

After running the demo, start the local API/dashboard with `make serve`, then open [http://127.0.0.1:8080](http://127.0.0.1:8080). It reads `artifacts/demo/report.json` (override with `REPORT_PATH`) and serves summary, finding filters, the latest report, and local review actions. The run-history endpoint reflects only the latest report.

## Finding behavior

- `amount_mismatch`: exact absolute difference in minor units for a successful provider capture.
- `unsupported_paid_status`: application success with a matched provider state other than succeeded.
- `provider_charge_not_recorded`: provider capture matched to an application payment that is not successful.
- `missing_provider_charge`: successful application payment absent from a complete provider snapshot.
- `unrecognized_provider_capture`: captured provider charge without a matching application payment.
- `duplicate_internal_credit`: multiple application records use one account-scoped provider payment reference.
- `ambiguous_provider_match` and `potential_duplicate_provider_capture`: multiple provider charges share the same account and payment reference; duplicate captures remain explicitly uncertain.
- `account_ownership_unverified`: account-to-tenant ownership is absent or conflicts, so the engine refuses to compare those records.
- `refund_not_recorded`, `refund_missing_provider`, and `refund_amount_mismatch`: compare successful refund records while preserving partial-refund amounts.
- `subscription_status_mismatch`: emitted only after the configured evaluator grace period; it does not infer entitlement correctness.

Duplicate webhook deliveries are not represented as money movements. Currency values are never aggregated across currencies. Findings are deterministic and have stable fingerprints for unchanged inputs. Matching uses exact provider payment references within the account and requires explicit account-to-tenant ownership mapping. Recent missing or not-yet-synchronized payments are held during the configured grace period.

## Architecture and limits

`internal/provider` owns provider-side fixture records and HTTP endpoints. `internal/reconcile` contains typed records and pure evaluation logic, with no HTTP or persistence dependency. `cmd/payment-reconciliation` collects a normalized snapshot, evaluates it, and exports reports. This process separation ensures the evaluator consumes adapter output instead of querying simulator records directly.

The simulator is in-memory and resets on restart. There is no PostgreSQL persistence in this version. The review endpoint appends local JSONL audit entries and assumes a trusted loopback operator; it is not suitable for exposure to an untrusted network. The result does not establish bank settlement, payout completion, or general-ledger correctness. The HTTP adapter has bounded retries but lacks jitter and durable resume cursors. Historical run storage and multi-user review authorization are not implemented.

## Development

```sh
make test       # unit and handler tests
make race       # race detector
make build
make demo
```

The demo fixture includes a failed provider attempt, amount mismatch, missing provider payment, duplicate internal credit, orphan capture, overlapping textual references across accounts, a missing application refund, and a stale subscription status mismatch. `fixtures/demo/expected.json` is an independent category manifest. The demo writes `verification.json` and fails if detected counts differ.

## License

MIT; see [LICENSE](LICENSE).
