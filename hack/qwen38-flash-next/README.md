# Qwen3.8-Flash-Next on Dynamo 1.4.2 + vLLM 0.31.0

Builds and deploys a Dynamo vLLM worker image for `Qwen/Qwen3.8-Flash-Next-FP8` (HF model type
`qwen4_exp`) for gaap-managed clusters running the Dynamo 1.4.2 platform.

Why a custom image: the stock `nvcr.io/nvidia/ai-dynamo/vllm-runtime:1.4.2` cannot load this
model, and the gaap controller fixes the worker command to `python3 -m dynamo.vllm`, so the
Dynamo wheel has to be baked into an upstream vLLM image rather than installed at pod start.

| | |
|---|---|
| Image | `registry.dev.rafay-edge.net/tf/dynamo-vllm-qwen38fn:1.4.2-vllm031.2` |
| Base | `vllm/vllm-openai:v0.31.0` (amd64 digest `sha256:a4a4c043…`) |
| Added | `ai-dynamo==1.4.2`, `ai-dynamo-runtime==1.4.2`, scheduler fix (`patches/`) |
| Platform | `linux/amd64` only (H200 nodes) |

Files in this directory:

- `Dockerfile` — the image. Build context is this directory.
- `patches/instrumented_scheduler.py` — ai-dynamo 1.4.2's file with the vLLM 0.31 fix
  (upstream #15643). Identical to `components/src/dynamo/vllm/instrumented_scheduler.py` on the
  RafaySystems/dynamo branch `1.4.2-vllm031`; keep the two in sync.
- `modeldeployment.yaml` — the gaap `ModelDeployment` spec for 8x H200.

## 1. Prerequisites

- Docker 24+ with BuildKit. ~25 GB free disk (9 GB base pull, ~20 GB unpacked).
- `docker login registry.dev.rafay-edge.net`.
- If the build host is arm64 (DGX Spark, Raspberry Pi), register the `tonistiigi/binfmt`
  QEMU handler. Do not use the distro `qemu-user-static`; Ubuntu 24.04's QEMU 8.2 segfaults
  in the emulated `pip install`.

  ```bash
  docker run --privileged --rm tonistiigi/binfmt:qemu-v9.2.2 --uninstall qemu-x86_64 || true
  docker run --privileged --rm tonistiigi/binfmt:qemu-v9.2.2 --install amd64
  docker run --rm --platform linux/amd64 alpine:3.20 uname -m   # x86_64
  ```

## 2. Build

From the repository root:

```bash
IMG=registry.dev.rafay-edge.net/tf/dynamo-vllm-qwen38fn:1.4.2-vllm031.2
docker build --platform linux/amd64 -t "$IMG" hack/qwen38-flash-next
```

The build prints the installed `ai-dynamo`, `ai-dynamo-runtime` and `vllm` versions and
`instrumented_scheduler.py: vLLM 0.31 compatible`. Native x86 build: ~1 min after the base pull.
Emulated on a DGX Spark: ~2 min.

To change the pinned versions: `--build-arg BASE_IMAGE=…` and `--build-arg DYNAMO_VERSION=…`.
The scheduler patch is specific to the ai-dynamo 1.4.x line; drop the `COPY` when moving to a
Dynamo release that includes upstream #15643.

## 3. Smoke test (no GPU)

```bash
docker run --rm --platform linux/amd64 --entrypoint python3 "$IMG" -c '
import dynamo.runtime, dynamo.llm, dynamo.vllm.main, dynamo.vllm.instrumented_scheduler, vllm
from vllm.transformers_utils.configs import Qwen4ExpConfig
print("ok", vllm.__version__)'
```

Expect `ok 0.31.0`. Under emulation this takes about 4 minutes (imports torch); warnings
about a missing CUDA runtime are normal.

## 4. Push and verify

```bash
docker push "$IMG"
crane config "$IMG" | jq '{arch: .architecture, layers: (.rootfs.diff_ids|length), user: .config.User, entrypoint: .config.Entrypoint}'
```

Expect `amd64`, 44 layers, user `root`, entrypoint `null`. The tag must be strict semver
(`1.4.2-vllm031.2` is); the Dynamo operator webhook derives the runtime version from the worker
tag and the gaap controller does not set `runtimeVersionOverride`.

If the push fails with `failed commit on ref … timeout awaiting response headers`, the registry
timed out committing a large base layer. Re-run the push; completed layers are skipped. Push
from a host with good upstream bandwidth (gx10-b4fb worked; Wi-Fi hosts did not).

## 5. Deploy

Apply `modeldeployment.yaml` to the existing `ModelDeployment` (gaap console or API). The
comments in the file explain each argument and what the controller injects on its own.

Requirements the file assumes:

- Model weights on a ReadOnlyMany PVC mounted at `/models` on every GPU node (`pvcName`).
- Nodes labelled `nvidia.com/gpu.product=NVIDIA-H200` with 8 GPUs and >= 256Gi free memory.
- The Dynamo 1.4.2 platform (operator, EPP, frontend image) already installed.

Expected startup, about 7 minutes after the image is on the node:

| Phase | Log line to look for |
|---|---|
| Config | `Using fp8 data type to store kv cache`, `Resolved Engram configuration: EngramConfig(cpu_offload=True, …)` |
| Kernels | `Using TRITON Fp8 MoE backend`, `Using FlashAttention version 3` |
| Weights | `Model loading took 18.1 GiB memory and ~115 seconds` (per rank) |
| KV cache | `Setting attention block size to 800 tokens`, `GPU KV cache size: 14,608,706 tokens` |
| Ready | `Registered base model '<name>' MDC`; EPP logs `Found model card via discovery … kv_cache_block_size=800` |

Then send a request through the gateway:

```bash
curl -sS https://<gateway>/v1/chat/completions \
  -H "Authorization: Bearer $API_KEY" -H "Content-Type: application/json" \
  -d '{"model":"<name>","max_tokens":64,"messages":[{"role":"user","content":"Say hello in one sentence."}]}'
```

## 6. Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `Transformers does not recognize this architecture (qwen4_exp)` | stock Dynamo image, or vLLM older than the model | use this image |
| `Qwen3.8-Flash-Next QSA requires a BF16 main KV cache` | vLLM older than 0.31 with `--kv-cache-dtype fp8` | use the 0.31.0 base, or drop the two fp8 flags |
| First request kills the engine: `'InstrumentedScheduler' object has no attribute 'skipped_waiting'`; gateway then returns `503 failed to find endpoint candidates` | unpatched ai-dynamo 1.4.2 on vLLM 0.31 | this image's `patches/` fix; the wrapper cannot be disabled from the ModelDeployment because the operator always sets `DYN_FORWARDPASS_METRIC_PORT` |
| `Using default MoE config. Performance might be sub-optimal! … E=64,N=640,device_name=NVIDIA_H200,dtype=fp8_w8a8,block_shape=[128,128].json` | no tuned Triton MoE config for this expert shape | generate it with vLLM's `benchmarks/kernels/benchmark_moe.py` on an H200 and `COPY` it into the image at that path |
| Pod memory at the limit, many major page faults | N-gram table in host RAM plus 8 worker processes | memory >= 256Gi (see `modeldeployment.yaml`) |
| `Found duplicate keys --tensor-parallel-size` | flag repeated in `arguments` | remove it; the controller injects it from the GPU count |
| QEMU `internal SIGSEGV` during `pip install` on an arm64 build host | distro QEMU | install the `tonistiigi/binfmt` handler (section 1) |

Notes on behaviour that is not a bug: vLLM re-aligns the KV block size for this hybrid
GDN/attention model regardless of `--block-size` (800 tokens with fp8 KV, 400 with bf16); the
worker publishes the real value in its model card and the EPP reads it from there. Every
restart pays ~3 minutes of torch.compile and CUDA graph capture because the compile cache is
on an emptyDir; a persistent `/root/.cache/vllm` would remove that but the gaap
`ModelDeployment` has no volume knob for it today.
