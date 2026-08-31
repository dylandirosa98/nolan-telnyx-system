.PHONY: test vet race fmt build compose-up compose-down
fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')
test:
	go test ./...
vet:
	go vet ./...
race:
	go test -race ./...
build:
	go build ./cmd/api ./cmd/worker
compose-up:
	docker-compose up --build
compose-down:
	docker-compose down -v
