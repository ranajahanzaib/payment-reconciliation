# Provider adapter

The HTTP simulator uses account-scoped bearer authentication, deterministic page order, bounded page size, time-window filters, and collection plus individual-record endpoints for charges, refunds, and subscriptions. The collector uses an HTTP timeout and returns incomplete status when collection fails. The offline demo adapter normalizes simulator fixtures through JSON serialization for environments that prohibit loopback sockets.

The adapter is not a production payment-provider integration. It has a request timeout, bounded exponential retries, and `Retry-After` support for HTTP 429/5xx responses. Jitter, update-window cursor recovery, raw evidence persistence, and provider-specific adapters remain future work.
