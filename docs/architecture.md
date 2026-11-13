# Architecture

`internal/reconcile` is a pure domain evaluator. The provider simulator exposes provider-owned records from `internal/provider`; the command collects those records through an adapter and passes a normalized snapshot to the evaluator. Application records and provider records are separate typed collections. This version keeps both in memory; PostgreSQL persistence is not implemented.

Money uses signed 64-bit integer minor units. Matching requires the exact provider reference and provider account. A duplicate candidate is uncertain and is never arbitrarily selected. A partial snapshot cannot prove a provider record is missing. No evaluation path writes financial state.

This prototype does not prove settlement, payout, or ledger correctness. It does not implement durable snapshots or historical run persistence. Review API actions append to a local JSONL audit file and are intended only for a trusted loopback environment, not a multi-user deployment.
