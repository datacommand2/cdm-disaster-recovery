.PHONY: build vendor test lint clean

VERSION ?= "unknown"

build:
	GOFLAGS='-mod=vendor' GOPRIVATE=10.1.1.220/** GOINSECURE=10.1.1.220/** CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o mirrord -v -a -ldflags "-extldflags '-static' -X 'main.version=$(VERSION)'" || exit 1;

vendor:
	GOFLAGS='-mod=vendor' GOPRIVATE=10.1.1.220/** GOINSECURE=10.1.1.220/** go mod tidy
	GOFLAGS='-mod=vendor' GOPRIVATE=10.1.1.220/** GOINSECURE=10.1.1.220/** go mod vendor

test:
	GOFLAGS='-mod=vendor' GOPRIVATE=10.1.1.220/** GOINSECURE=10.1.1.220/** go test -p=1 -v -coverprofile=coverage.out $$(go list ./... | grep -v /vendor/)
	GOFLAGS='-mod=vendor' GOPRIVATE=10.1.1.220/** GOINSECURE=10.1.1.220/** go tool cover -func=coverage.out
	GOFLAGS='-mod=vendor' GOPRIVATE=10.1.1.220/** GOINSECURE=10.1.1.220/** go tool cover -html=coverage.out -o coverage.html
	GOFLAGS='-mod=vendor' GOPRIVATE=10.1.1.220/** GOINSECURE=10.1.1.220/** gocover-cobertura < coverage.out > coverage.xml

lint:
	go fmt $$(go list ./... | grep -v /vendor/)
	go vet $$(go list ./... | grep -v /vendor/)
	GOFLAGS='-mod=vendor' GOPRIVATE=10.1.1.220/** GOINSECURE=10.1.1.220/** golint -set_exit_status $$(go list ./... | grep -v /vendor/) || exit 1

clean:
	@rm -f coverage.out coverage.html coverage.xml mirrord
