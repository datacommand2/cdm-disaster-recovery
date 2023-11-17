.PHONY: build proto vendor test lint clean

VERSION ?= "unknown"

build: proto
	GOFLAGS='-mod=vendor' CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -v -a -ldflags "-extldflags '-static' -X 'main.version=$(VERSION)'" || exit 1;

proto:
	@for FILE in proto/*.proto; do \
	    protoc -I$$GOPATH -I ${PWD}/vendor --proto_path=${PWD} --micro_out=. --go_out=. $$FILE || exit 1; \
	    echo compiled: $$FILE; \
	done

vendor:
	GOFLAGS='-mod=vendor' go mod tidy
	GOFLAGS='-mod=vendor' go mod vendor

test:
	GOFLAGS='-mod=vendor' go test -p 1 -v -race -coverprofile=coverage.out $$(go list ./... | grep -v /vendor/)
	GOFLAGS='-mod=vendor' go tool cover -func=coverage.out
	GOFLAGS='-mod=vendor' go tool cover -html=coverage.out -o coverage.html
	GOFLAGS='-mod=vendor' gocover-cobertura < coverage.out > coverage.xml

lint:
	go fmt $$(go list ./... | grep -v /vendor/)
	go vet $$(go list ./... | grep -v /vendor/)
	GOFLAGS='-mod=vendor' golint $$(go list ./... | grep -v /vendor/) || exit 1

clean:
	@rm -f coverage.out coverage.html coverage.xml manager
