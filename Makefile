.PHONY: build install test lint docs fmt snapshot clean

NAME     := artisanvirt
BINARY   := terraform-provider-$(NAME)
VERSION  ?= 0.0.1
OS_ARCH  := $(shell go env GOOS)_$(shell go env GOARCH)
PLUGINS  := $(HOME)/.terraform.d/plugins/registry.terraform.io/developerprince/$(NAME)/$(VERSION)/$(OS_ARCH)

build:
	go build -o bin/$(BINARY) .

# Installs into the local filesystem mirror so `terraform init` finds
# developerprince/artisanvirt version $(VERSION) without the registry.
install:
	go build -ldflags="-X main.version=$(VERSION)" -o $(PLUGINS)/$(BINARY)_v$(VERSION) .

test:
	go vet ./...
	go test -timeout 20m ./...

lint:
	test -z "$$(gofmt -l .)"
	go vet ./...
	terraform fmt -recursive -check examples

fmt:
	gofmt -w .
	terraform fmt -recursive examples

# Regenerate docs/ (what the Terraform Registry renders) and validate it.
docs:
	go generate ./...
	go tool tfplugindocs validate --provider-name $(NAME)

# Local unsigned build of every platform, like the CI snapshot job.
snapshot:
	goreleaser release --snapshot --clean --skip=sign

clean:
	rm -rf bin dist
