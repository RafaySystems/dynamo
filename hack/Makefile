# Build the fixes frontend image: the fixed EPP binary overlaid on the
# upstream monolithic dynamo-frontend image.
#
# Two steps:
#   1) epp-image      — compile the EPP from source (hack/Dockerfile.epp)
#   2) frontend-image — overlay /epp onto the stock frontend (hack/Dockerfile.frontend-overlay)
#
# Usage (native linux/amd64 host, plain docker — no buildx needed):
#   make -C hack push
# Override anything on the command line, e.g.:
#   make -C hack push VERSION=1.4.2 REGISTRY=registry.dev.rafay-edge.net/tf

REPO_ROOT   := $(abspath $(dir $(lastword $(MAKEFILE_LIST)))/..)

PLATFORM    ?= linux/amd64
VERSION     ?= 1.4.2
REGISTRY    ?= registry.dev.rafay-edge.net/tf

# Upstream frontend image the fix is overlaid onto.
BASE_IMAGE  ?= nvcr.io/nvidia/ai-dynamo/dynamo-frontend:$(VERSION)
# Local intermediate EPP image (not pushed).
EPP_IMAGE   ?= dynamo-epp:$(VERSION)-fixes
# Final pushed image (drop-in replacement for the frontend/EPP pod).
IMAGE       ?= $(REGISTRY)/dynamo-frontend:$(VERSION)-fixes

COMMIT_SHA  := $(shell git -C $(REPO_ROOT) rev-parse HEAD 2>/dev/null)
BUILD_REF   := $(shell git -C $(REPO_ROOT) rev-parse --abbrev-ref HEAD 2>/dev/null)

export DOCKER_BUILDKIT := 1

.PHONY: all push frontend-image epp-image print

## all: build the EPP image, overlay it, and push (default)
all: push

## epp-image: compile the fixed EPP binary into a local image
epp-image:
	docker build \
	  --platform=$(PLATFORM) \
	  -f $(REPO_ROOT)/hack/Dockerfile.epp \
	  --build-arg COMMIT_SHA=$(COMMIT_SHA) \
	  --build-arg BUILD_REF=$(BUILD_REF) \
	  -t $(EPP_IMAGE) \
	  $(REPO_ROOT)

## frontend-image: overlay the fixed /epp onto the upstream frontend image
frontend-image: epp-image
	docker build \
	  --platform=$(PLATFORM) \
	  -f $(REPO_ROOT)/hack/Dockerfile.frontend-overlay \
	  --build-arg BASE_IMAGE=$(BASE_IMAGE) \
	  --build-arg EPP_IMAGE=$(EPP_IMAGE) \
	  -t $(IMAGE) \
	  $(REPO_ROOT)/hack

## push: build and push the final image
push: frontend-image
	docker push $(IMAGE)

## print: show the resolved image names
print:
	@echo "PLATFORM   = $(PLATFORM)"
	@echo "BASE_IMAGE = $(BASE_IMAGE)"
	@echo "EPP_IMAGE  = $(EPP_IMAGE)"
	@echo "IMAGE      = $(IMAGE)"
	@echo "COMMIT_SHA = $(COMMIT_SHA)"
	@echo "BUILD_REF  = $(BUILD_REF)"
