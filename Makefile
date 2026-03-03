VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

IMAGE_TAG ?= $(VERSION)

LDFLAGS := -ldflags "-X github.com/tiwillia/sankey-scorecard/cmd.Version=$(VERSION) \
	-X github.com/tiwillia/sankey-scorecard/cmd.Commit=$(COMMIT) \
	-X github.com/tiwillia/sankey-scorecard/cmd.BuildDate=$(BUILD_DATE)"

.PHONY: build install test test-integration lint test-all build-image deploy deploy-teardown

build:
	go build $(LDFLAGS) -o sankey-scorecard ./cmd/sankey-scorecard

install:
	go install $(LDFLAGS) ./cmd/sankey-scorecard

test:
	go test ./pkg/... -coverprofile=coverage.out -covermode=atomic

test-integration:
	go test ./tests/... -tags=integration -coverprofile=coverage.out -covermode=atomic

lint:
	golangci-lint run ./...

test-all: lint test test-integration

build-image:
	podman build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-f Containerfile \
		-t sankey-scorecard:$(IMAGE_TAG) .

deploy:
ifndef NAMESPACE
	$(error NAMESPACE is required. Usage: make deploy NAMESPACE=my-namespace)
endif
	NAMESPACE=$(NAMESPACE) IMAGE_TAG=$(IMAGE_TAG) bash deploy/openshift/deploy.sh

deploy-teardown:
ifndef NAMESPACE
	$(error NAMESPACE is required. Usage: make deploy-teardown NAMESPACE=my-namespace)
endif
	oc delete route sankey-scorecard -n $(NAMESPACE) --ignore-not-found
	oc delete service sankey-scorecard -n $(NAMESPACE) --ignore-not-found
	oc delete deployment sankey-scorecard -n $(NAMESPACE) --ignore-not-found
	oc delete deployment sankey-scorecard-postgres -n $(NAMESPACE) --ignore-not-found
	oc delete service sankey-scorecard-postgres -n $(NAMESPACE) --ignore-not-found
	oc delete pvc sankey-scorecard-postgres -n $(NAMESPACE) --ignore-not-found
	@echo "Note: Secrets 'sankey-scorecard-jira' and 'sankey-scorecard-db' were not deleted. Remove manually if needed."
