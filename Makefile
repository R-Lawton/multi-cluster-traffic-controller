
# Image URL to use all building/pushing image targets
TAG ?= latest

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.25.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

include ./hack/make/*.make

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: clean
clean: ## Clean up temporary files.
	-rm -rf ./bin/*
	-rm -rf ./tmp
	-rm -rf ./config/**/charts

.PHONY: gateway-manifests
gateway-manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role paths="./pkg/controllers/gateway" output:rbac:artifacts:config=config/rbac

.PHONY: manifests
manifests: gateway-manifests

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: imports ## Run go fmt against code.
	gofmt -s -w .

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint against code.
	golangci-lint run ./...

.PHONY: imports
imports: openshift-goimports ## Run openshift goimports against code.
	$(OPENSHIFT_GOIMPORTS) -m github.com/Kuadrant/multicluster-gateway-controller -i github.com/kuadrant

.PHONY: test-unit
test-unit: manifests generate fmt vet envtest ## Run unit tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $(shell find ./pkg/_internal -mindepth 1  -type d) ./...  -tags=unit -coverprofile cover-unit.out

.PHONY: test-integration
test-integration: ginkgo manifests generate fmt vet envtest ## Run integration tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(GINKGO) -tags=integration -v --focus "${FOCUS}" ./test/gateway_integration

.PHONY: test
test: test-unit test-integration ## Run tests.

.PHONY: test-e2e
test-e2e: ginkgo
	$(GINKGO) -tags=e2e -v ./test/e2e

.PHONY: local-setup
local-setup: local-setup-kind local-setup-mgc ## Setup multi cluster traffic controller locally using kind.
	$(info Setup is done! Enjoy)
	$(info Consider using local-setup-kind or local-setup-mgc targets to separate kind clusters creation and deployment of resources)

.PHONY: local-setup-kind
local-setup-kind: kind ## Setup kind clusters for multi cluster traffic controller.
	./hack/local-setup-kind.sh

.PHONY: local-setup-mgc
local-setup-mgc: kustomize helm yq operator-sdk clusteradm ## Setup multi cluster traffic controller locally onto kind clusters.
	./hack/local-setup-mgc.sh

.PHONY: local-cleanup
local-cleanup: kind ## Cleanup kind clusters created by local-setup
	./hack/local-cleanup-kind.sh
	$(MAKE) clean

.PHONY: local-cleanup-mgc
local-cleanup-mgc: ## Cleanup MGC from kind clusters
	./hack/local-cleanup-mgc.sh

.PHONY: build
build: build-gateway-controller ## Build all binaries.

##@ Deployment
ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: thanos-manifests
thanos-manifests: ./hack/thanos/thanos_build.sh ./hack/thanos/thanos.jsonnet
	./hack/thanos/thanos_build.sh
