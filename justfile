watch:
    air -c .air.toml

alias w := watch

dev:
    @go run ./dev/main.go

alias d := dev

test:
    # -count=2 - run each test twice
    # -race - run tests with race detection
    # -shuffle=on - shuffle tests to catch flakiness
    # -cover - show test coverage
    # -covermode=atomic - thread-safe coverage for race testing
    @go test ./cmd/... ./pkg/... -count=2 -race -shuffle=on -cover -covermode=atomic

alias t := test

lint:
    @golangci-lint run ./cmd/... ./pkg/...

alias l := lint

completion:
    @go run main.go completion zsh > "_tobi"
    @sudo mv _tobi /usr/share/zsh/site-functions/

install: completion
    @go install .

alias i := install