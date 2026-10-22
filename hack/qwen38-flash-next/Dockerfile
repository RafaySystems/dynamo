# syntax=docker/dockerfile:1.10.0
#
# Dynamo vLLM worker image for Qwen3.8-Flash-Next (HF model_type `qwen4_exp`).
#
# Upstream vLLM 0.32.0 release image + the ai-dynamo 1.4.2 wheel + one compatibility
# fix for ai-dynamo's forward-pass-metrics scheduler on vLLM >= 0.31.
#
# Build context is THIS directory:
#   docker build --platform linux/amd64 -t <registry>/dynamo-vllm-qwen38fn:1.4.2-vllm032.1 hack/qwen38-flash-next
#
# See README.md in this directory for prerequisites, verification and deployment.

# vllm/vllm-openai:v0.32.0, linux/amd64 manifest, pinned by digest for reproducibility.
# vLLM 0.31.0 is the first release that registers Qwen4Exp; 0.32.0 is required for disaggregated
# serving (vllm#59997: 0.31 truncates the PLE short-conv state in the NIXL transfer -> garbage output).
# The vLLM 0.31.0 base (aggregated only) was sha256:a4a4c0437bf7240089da5f08aa370c4aee17ae5290f7a3b468825ee26c4c3a6b.
ARG BASE_IMAGE=vllm/vllm-openai@sha256:6ee133356d49ed24cee03dd0cda3d3980279a11c107ce3e752c89e76540f41c9
ARG DYNAMO_VERSION=1.4.2

FROM ${BASE_IMAGE}
ARG DYNAMO_VERSION
USER root

# The bare `ai-dynamo` package (no [vllm] extra): the extra would pin and reinstall vLLM.
RUN pip install --no-cache-dir "ai-dynamo==${DYNAMO_VERSION}" \
    && pip show ai-dynamo ai-dynamo-runtime vllm | grep -E '^(Name|Version)'

# ai-dynamo 1.4.2 targets vLLM 0.26. Its InstrumentedScheduler reads the scheduler queue
# `skipped_waiting`, which vLLM 0.31 renamed to `kv_holding_waiting` (still the name in 0.32); without this fix the
# engine dies on the first request. patches/instrumented_scheduler.py is the 1.4.2 file with
# upstream commit 395f02405c (#15643) applied; the same file lives at
# components/src/dynamo/vllm/instrumented_scheduler.py on the RafaySystems/dynamo
# branch 1.4.2-vllm031.
COPY patches/instrumented_scheduler.py /usr/local/lib/python3.12/dist-packages/dynamo/vllm/instrumented_scheduler.py
RUN python3 - <<'PY'
import ast, pathlib
p = pathlib.Path("/usr/local/lib/python3.12/dist-packages/dynamo/vllm/instrumented_scheduler.py")
src = p.read_text()
ast.parse(src)
assert "kv_holding_waiting" in src and "from itertools import chain, count" in src, "scheduler fix not applied"
print("instrumented_scheduler.py: vLLM >= 0.31 compatible")
PY

# The upstream image's ENTRYPOINT is `vllm serve`; Dynamo runs `python3 -m dynamo.vllm`.
ENTRYPOINT []
WORKDIR /workspace
