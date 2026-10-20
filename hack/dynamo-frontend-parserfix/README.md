# dynamo-frontend 1.4.2 with the frontend-crates#408 parser fix

Builds `registry.dev.rafay-edge.net/tf/dynamo-frontend:1.4.2-fixes-parserfix`: the Dynamo 1.4.2
frontend image (frontend, EPP and Python wheels) from RafaySystems/dynamo `v1.4.2-fixes`, with the
tool-call parser fix from [ai-dynamo/frontend-crates#408](https://github.com/ai-dynamo/frontend-crates/pull/408)
("re-jail a complete start marker in trailing content") backported onto `dynamo-parsers` 7.0.1.

| | |
|---|---|
| Source branch | RafaySystems/dynamo `1.4.2-fixes-parserfix` (= `v1.4.2-fixes` @ `a11c63c0` + this change) |
| Parser source | RafaySystems/frontend-crates `test/pr408-port-7.0.1` @ `2062845874ba13b3ddd974c94f933c17acf1caea` |
| Image | `registry.dev.rafay-edge.net/tf/dynamo-frontend:1.4.2-fixes-parserfix` |
| Platform | `linux/amd64` |

## How the fix gets into the image

`dynamo-parsers` is a crates.io dependency (source repo `ai-dynamo/frontend-crates`). This branch
overrides it with a `[patch.crates-io]` entry in both Cargo workspaces:

- `Cargo.toml` (used by the EPP's `libdynamo_llm` FFI library)
- `lib/bindings/python/Cargo.toml` (used by the `ai-dynamo-runtime` wheel the frontend runs)

`dynamo-protocols` is patched to the same commit, so the parser and `lib/llm` share one copy of its
types. Both `Cargo.lock` files pin the git commit; the build uses `--locked`, so it cannot drift.

`Dockerfile` in this directory is the complete build: the repo's rendered 1.4.2 frontend build plus
in-file stages that compile the Dynamo EPP (and its Go SBOM). It needs no prebuilt images.

## 1. Prerequisites

- A native **x86_64** Docker host with BuildKit. The build compiles the whole Rust workspace twice
  (wheel and EPP); under QEMU on an arm64 host it is impractically slow.
- ~60 GB free disk, network access to crates.io, GitHub, PyPI, nvcr.io and quay.io.
- `docker login registry.dev.rafay-edge.net`.

## 2. Build

```bash
git clone -b 1.4.2-fixes-parserfix https://github.com/RafaySystems/dynamo.git
cd dynamo
IMG=registry.dev.rafay-edge.net/tf/dynamo-frontend:1.4.2-fixes-parserfix
docker build --platform linux/amd64 --progress=plain \
  -f hack/dynamo-frontend-parserfix/Dockerfile \
  --build-arg EPP_GIT_COMMIT=$(git rev-parse HEAD) \
  -t "$IMG" . 2>&1 | tee build.log
```

The build context must be the repository root.

## 3. Verify

The patched parser must be what got compiled. Every `Compiling dynamo-parsers` line in the log
(the wheel build and the EPP build each print one) must name the frontend-crates git source, not
crates.io:

```bash
grep 'Compiling dynamo-parsers' build.log
# ... Compiling dynamo-parsers v7.0.1 (https://github.com/RafaySystems/frontend-crates.git?rev=2062845874ba13b3ddd974c94f933c17acf1caea#20628458)
```

The EPP carries the branch build stamp, and the Python runtime is 1.4.2:

```bash
docker run --rm --entrypoint sh "$IMG" -c \
  'grep -a -oE "version\.(GitVersion|GitCommit)=[A-Za-z0-9._-]+" /epp | sort -u'
docker run --rm --entrypoint /opt/dynamo/venv/bin/python "$IMG" -c \
  'import importlib.metadata as m; print(m.version("ai-dynamo"), m.version("ai-dynamo-runtime"))'
```

Expect `GitVersion=1.4.2-fixes-parserfix`, the commit you built, and `1.4.2 1.4.2`.

The fix's own unit tests live in frontend-crates:

```bash
git clone -b test/pr408-port-7.0.1 https://github.com/RafaySystems/frontend-crates.git
cd frontend-crates && cargo test -p dynamo-parsers --test jail
```

## 4. Push

```bash
docker push "$IMG"
```

Point the gaap `ModelDeployment` at it with `spec.dynamoDeploymentSpec.frontendImage`; the same
image serves the frontend sidecar and the EPP.

## Changing the parser revision

1. Push the new commit to RafaySystems/frontend-crates.
2. Update `rev` for both `dynamo-parsers` and `dynamo-protocols` in `Cargo.toml` and
   `lib/bindings/python/Cargo.toml`.
3. Refresh both lockfiles and type-check:

   ```bash
   cargo update -p dynamo-parsers -p dynamo-protocols
   (cd lib/bindings/python && cargo update -p dynamo-parsers -p dynamo-protocols)
   cargo check --locked -p dynamo-llm          # add --no-default-features on macOS
   ```

4. Rebuild (section 2). Drop the `[patch.crates-io]` entries entirely once dynamo moves to a
   `dynamo-parsers` release that includes #408.

## Regenerating the Dockerfile

Only needed if `container/templates/` changes on this branch. Run
`python3 container/render.py --framework=dynamo --target=frontend --output-short-filename`, then
re-apply the two edits listed in the Dockerfile header: replace `FROM ${EPP_IMAGE} AS epp` with the
`epp_*` stages, and point the licenses stage's two `.epp-sbom/` COPYs at `epp_go_builder`.
