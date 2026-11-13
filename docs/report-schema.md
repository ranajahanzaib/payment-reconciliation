# Report schema

JSON reports use schema version `payment-reconciliation/v1`. Top-level fields include generation time, coverage completeness/errors and normalized records, provider/application record counts, findings, and fixture verification. A finding includes stable fingerprint, type, severity, confidence, tenant/account scope, matched record IDs, exact impact in minor units, explanation, and recommendation.

CSV exports use one row per finding and preserve integer minor units and original currency. HTML output escapes finding text and shows coverage and supporting summaries. The report does not aggregate exposure across currencies. A duplicate internal credit finding reports one transaction amount as an estimate rather than summing duplicate notification events.
