APP := node-reaper

SHELL=/bin/bash

BINDIR := $(CURDIR)/bin
GOARCH := amd64
LDFLAGS := -s -w
CGO_ENABLED := 0

GITSHORTHASH=$(shell git log -1 --pretty=format:%h)

GO ?= go
GOOS ?= linux
GOGC ?= 2000
GOTAGS ?=
VERSION ?= $(GITSHORTHASH)
GOTESTFLAGS ?=

NODEREAPERCONFIG ?= $(CURDIR)/node-reaper.yaml

# Insert build metadata into binary
LDFLAGS += -X main.version=$(VERSION)
LDFLAGS += -X main.gitCommit=$(GITSHORTHASH)

define MUTATION_TEST
pkg/drainer/drain.go \
pkg/nodereaper/cluster_health_calculator.go \
pkg/nodereaper/nodereaper.go
endef

.PHONY: test
test:
	$(GO) test -v $(GOTESTFLAGS) $(shell go list ./... | grep -v /vendor/)

.PHONY: test-mutation
test-mutation:
	go-mutesting $(MUTATION_TEST)

.PHONY: binary
binary: bin

.PHONY: bin
bin: bin/node-reaper

bin/node-reaper:
	GOGC=$(GOGC) go build -ldflags "$(LDFLAGS)" -tags "$(GOTAGS)" -o bin/node-reaper cmd/nodereaper/main.go

.PHONY: plugins
plugins: bin/plugins/aws.so

bin/plugins/aws.so:
	GOGC=$(GOGC) go build -ldflags "$(LDFLAGS)" -tags "$(GOTAGS)" -o bin/plugins/aws.so -buildmode=plugin pkg/plugins/aws/main.go

.PHONY: build-base
build-base:
	@docker images node-reaper-build:$(VERSION) \
		| grep -q node-reaper-build \
		|| docker build --no-cache -t node-reaper-build:$(VERSION) -f dockerfiles/Dockerfile.build .

.PHONY: build
build: build-base dep
	docker run --rm \
		--mount type=bind,source=$(CURDIR),dst=/go/src/github.com/HotelsDotCom/node-reaper,consistency=delegated \
		--mount type=volume,source=node-reaper-build-cache,dst=/cache \
		--tmpfs /tmp \
		-e VERSION=$(VERSION) \
		-e GOCACHE=/cache \
		node-reaper-build:$(VERSION) \
		clean-bin plugins bin

.PHONY: package
package: build
	! (docker images node-reaper:$(VERSION) | grep -q node-reaper) && \
	docker build -t node-reaper:$(VERSION) -f dockerfiles/Dockerfile.package .

.PHONY: run
run:
	docker run -ti \
		-v $(NODEREAPERCONFIG):/node-reaper.yaml \
		-v ${KUBECONFIG}:/kubeconfig \
		-v $(CURDIR)/bin/plugins:/plugins \
		--env-file <(env | grep AWS_) \
		node-reaper:$(VERSION) \
		  -kubeconfig=/kubeconfig

.PHONY: clean
clean: clean-bin
	@docker rm node-reaper > /dev/null 2>&1 || true
	@docker rmi --force $(shell docker images node-reaper:* -q) > /dev/null 2>&1 || true

.PHONY: clean-build-cache
clean-build-cache:
	@docker volume ls -q | grep node-reaper-build | xargs docker volume rm || true

.PHONY: clean-bin
clean-bin:
	@rm -rf $(BINDIR)

.PHONY: clean-base
clean-base:
	@docker rmi --force $(shell docker images node-reaper-build:* -q) || true

.PHONY: clean-dep-go-mutesting
clean-dep-go-mutesting:
	@docker rm go-mutesting || true

.PHONY: clean-all
clean-all: clean clean-base clean-build-cache

.PHONY: dep
dep:
	$(GO) get github.com/golang/dep/cmd/dep
	dep ensure -vendor-only

.PHONY: test-dep
test-dep: dep-go-mutesting

.PHONY: dep-go-mutesting
dep-go-mutesting: clean-dep-go-mutesting
	docker run -it --name go-mutesting golang:1.9 /bin/bash -c "go get -t -v github.com/zimmski/go-mutesting/...; cd /go/src/github.com/zimmski/go-mutesting; GOOS=darwin go build cmd/go-mutesting/main.go"
	docker cp go-mutesting:/go/src/github.com/zimmski/go-mutesting/main $(shell go env GOPATH)/bin/go-mutesting
	docker rm go-mutesting

.PHONY: licence
licence:
	@scripts/add_licence_headers.sh
