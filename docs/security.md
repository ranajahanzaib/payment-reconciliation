# Security

The simulator requires a bearer key mapped to a provider account. It rejects collection requests without an account and rejects unknown accounts. `provider-sim` refuses to start unless `PROVIDER_API_KEY` is set. Generate a key for local use with `openssl rand -hex 32`; do not commit it or put it in shell history.

The review API binds to `127.0.0.1:8080` by default. It writes append-only JSONL audit events with mode `0600`. It has no user authentication, authorization, CSRF defense, or multi-user identity provider. Do not bind it to a public or shared network. `REVIEW_ACTOR` is an operator label, not authenticated identity.

Provider snapshots and generated reports may contain financial references. Keep artifacts local and apply appropriate access controls. The system never executes financial repairs. This prototype has no PostgreSQL role separation because PostgreSQL persistence is not implemented.
