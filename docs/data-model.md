# Data model

The prototype uses typed in-memory application `Payment`, `ApplicationRefund`, and `ApplicationSubscription` records and a provider `Snapshot` containing charges, refunds, subscriptions, completeness, errors, and account-to-tenant ownership. Reconciliation requires the account owner mapping before comparison. Money fields are signed `int64` minor units; currencies remain attached to each value. PostgreSQL persistence is outside the current release scope.

Provider identity is scoped by provider account. Exact payment references are required for payment matching; multiple candidates produce an uncertain finding. Refund IDs and subscription IDs are likewise matched only within an account. Duplicate delivery events are not modeled as transactions.
