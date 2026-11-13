# Reconciliation rules

The implemented rules are documented in the README. They compare captured/succeeded charges, application payment state, amount, currency, and duplicate references. Amounts are exact integers in the original currency. Captures without an application match and successful application payments without provider matches are reported only when snapshot completeness allows that conclusion.

Refund checks distinguish provider refunds absent from the application, application refunds absent from a complete provider snapshot, and amount/currency mismatches. Partial amounts are reported exactly; a refund does not alter payment or invoice status. Subscription status mismatches wait until the newest side's update is older than the configured grace period. Findings recommend review and do not change entitlements. No findings trigger automatic repairs.
