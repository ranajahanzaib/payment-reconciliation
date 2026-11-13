# Testing

Run `make test`, `make race`, and `make build`. The Go tests cover exact amount differences, stable finding fingerprints, incomplete-import suppression, account scoping, tenant ownership, grace periods, duplicate internal credits, uncertain duplicate provider captures, refund and subscription rules, simulator authentication, pagination, record detail, and time filters.

`make demo` executes the 1,000-charge scenario, compares actual findings with `fixtures/demo/expected.json`, writes a machine-readable `verification.json`, and repeats evaluation to check stable fingerprints. PostgreSQL integration and multi-process concurrency tests are outside the current suite. Simulator HTTP handlers are exercised directly with `httptest.ResponseRecorder`; tests do not require a running network service.
