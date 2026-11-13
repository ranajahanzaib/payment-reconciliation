.PHONY: setup test race build demo serve provider-sim clean
setup:
	go mod download
test:
	go test ./...
race:
	go test -race ./...
build:
	go build -o bin/payment-reconciliation ./cmd/payment-reconciliation
demo:
	go run ./cmd/payment-reconciliation demo
serve:
	go run ./cmd/payment-reconciliation serve
provider-sim:
	go run ./cmd/payment-reconciliation provider-sim
clean:
	rm -rf bin artifacts/demo
