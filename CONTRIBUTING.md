# Contributing

Bug reports, documentation updates, and code contributions are welcome. For a defect report, include the Go version, operating system, command or API request, expected behavior, and observed output. Do not include real payment data, provider credentials, or customer information.

## Development setup

```sh
git clone https://github.com/ranajahanzaib/payment-reconciliation.git
cd payment-reconciliation
make test
make demo
```

Before opening a pull request, run:

```sh
gofmt -w ./cmd ./internal
go vet ./...
go test ./...
go test -race ./...
```

For reconciliation changes, add focused tests for matching boundaries, snapshot completeness, exact monetary differences, and stable finding identity. Keep demo expected outcomes in `fixtures/demo/expected.json` synchronized with intentional fixture changes. Document behavior or configuration changes in the README and relevant `docs/` page.

Open an issue for a substantial change before starting a large implementation. Pull requests should explain the behavior changed, include test results, and identify relevant limitations.
