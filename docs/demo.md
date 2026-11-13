# Demo

Run `make demo`. The command creates exactly 1,000 provider charge attempts in a deterministic fixture, collects through the simulator HTTP API when loopback TCP is available (otherwise its offline fixture adapter), reconciles application records, checks the independent expected-results manifest, and repeats evaluation.

Reports are `artifacts/demo/report.json`, `report.csv`, `report.html`, and `verification.json`. The fixture covers a failed provider attempt, amount mismatch, missing provider charge, duplicate internal credit, captured provider charge with no application match, overlapping references across accounts, missing application refund, and stale subscription state. The recent cross-tenant collision is held during the grace period.

To inspect the report, run `make serve` and open `http://127.0.0.1:8080`. This dashboard reads the latest report file; it is not a persistent run browser.
