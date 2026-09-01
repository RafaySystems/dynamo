# hack/ — fixes frontend image build

Builds a drop-in `dynamo-frontend` image carrying the EPP fixes on this branch:
the KV-router tool-call fix (#11991) and the prefill reservation series (see
`deploy/inference-gateway/epp/pkg/plugins/` and
`lib/llm/src/kv_router/prefill_router/`).

The official `dynamo-frontend` image is monolithic — it serves the frontend,
workers, and the EPP (entrypoint `/epp`) from one image. Only `/epp` changes for
these fixes, so we compile the fixed EPP and **overlay** it onto the stock
frontend image rather than rebuilding the whole thing. Every other role stays
identical to the upstream release.

## Files

- `Dockerfile.epp` — compiles the EPP binary from a single build context (repo
  root), so a plain `docker build` works (no buildx / named contexts). Native
  `linux/amd64` build.
- `Dockerfile.frontend-overlay` — copies the built `/epp` onto the upstream
  frontend image (`BASE_IMAGE`).
- `Makefile` — orchestrates: `epp-image` → `frontend-image` → `push`.

## Usage

On a native `linux/amd64` host with Docker (BuildKit) and registry access:

```bash
docker login registry.dev.rafay-edge.net
make -C hack push
```

Override defaults as needed:

```bash
make -C hack push \
  VERSION=1.4.2 \
  REGISTRY=registry.dev.rafay-edge.net/tf \
  BASE_IMAGE=nvcr.io/nvidia/ai-dynamo/dynamo-frontend:1.4.2
```

`make -C hack print` shows the resolved image names without building.

Then point the EPP pod's `extension.image` (or the shared frontend tag) at the
pushed image, e.g. `registry.dev.rafay-edge.net/tf/dynamo-frontend:1.4.2-fixes`.

## Verify

```bash
docker run --rm --entrypoint /epp \
  registry.dev.rafay-edge.net/tf/dynamo-frontend:1.4.2-fixes --help   # prints EPP flags
```
