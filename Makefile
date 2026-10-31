GOENV = GOWORK=off GOFLAGS='-trimpath -p=2' GOMAXPROCS=3
GO = env $(GOENV) nice -n 15 go
.PHONY: build check
build:
	$(GO) build -o o-questions .
check:
	@test -z "$$(gofmt -l .)"
	$(GO) vet ./...
	$(GO) test -race ./...
