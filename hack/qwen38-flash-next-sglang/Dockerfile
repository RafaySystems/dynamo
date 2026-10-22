# syntax=docker/dockerfile:1.10.0
#
# Feasibility image: Dynamo 1.4.2 SGLang worker for Qwen3.8-Flash-Next (HF model_type `qwen4_exp`).
#
# SGLang supports qwen4_exp from v0.5.20 (PD state transfer for the model: sgl-project/sglang#36651),
# while Dynamo 1.4.2 pins SGLang 0.5.16. This image pairs the two to measure that gap.
#
#   docker build --platform linux/amd64 -t <registry>/dynamo-sglang-qwen38fn:1.4.2-sglang0522.1 hack/qwen38-flash-next-sglang

# lmsysorg/sglang:v0.5.22 (CUDA 13.0), linux/amd64 manifest, pinned by digest.
ARG BASE_IMAGE=lmsysorg/sglang@sha256:5fc09edb6a0bb1d5b6992b543723c66d08f86d7a507e951e87e0d05db4ac766e
ARG DYNAMO_VERSION=1.4.2

FROM ${BASE_IMAGE}
ARG DYNAMO_VERSION
USER root

# Bare `ai-dynamo` (no [sglang] extra): the extra would pin and reinstall SGLang 0.5.16.
RUN pip install --no-cache-dir "ai-dynamo==${DYNAMO_VERSION}" \
    && pip show ai-dynamo ai-dynamo-runtime sglang | grep -E '^(Name|Version)'

# The gaap controller runs `python3 -m dynamo.sglang`; clear any upstream entrypoint.
ENTRYPOINT []
WORKDIR /workspace
