# Qwen3.8-Flash-Next on Dynamo 1.4.2 + SGLang 0.5.22 (feasibility, not validated)

Status: built once, not deployed. Set aside after the vLLM 0.32.0 image
(`hack/qwen38-flash-next`) passed disaggregated parity.

Why it was considered: SGLang has model-specific prefill/decode state transfer for
Qwen3.8-Flash-Next (PLE short-conv and n-gram state, QSA pending state and compressed K;
sgl-project/sglang#36651, in v0.5.20+), at a time when vLLM 0.31 corrupted that state.

Gap: SGLang supports `qwen4_exp` from v0.5.20, while Dynamo 1.4.2 pins SGLang 0.5.16
(1.5.1: 0.5.18, main: 0.5.21). This image pairs ai-dynamo 1.4.2 with SGLang 0.5.22.

```bash
docker build --platform linux/amd64 -t <registry>/dynamo-sglang-qwen38fn:1.4.2-sglang0522.1 hack/qwen38-flash-next-sglang
docker run --rm --platform linux/amd64 -v $PWD/hack/qwen38-flash-next-sglang/compat.py:/tmp/compat.py:ro \
  --entrypoint python3 <image> /tmp/compat.py
```

`compat.py` checks, without a GPU: every `dynamo.sglang` submodule imports, every SGLang symbol
Dynamo references resolves, the flags the gaap controller injects (`--disaggregation-mode`,
`--disaggregation-transfer-backend nixl`, `--page-size`, `--tp-size`, `--kv-events-config`) parse,
and the model plus its PD state transfer are present. It has not been run yet.

Open SGLang issues for this model to check before use: #37326 (MTP acceptance decays over
uptime), #37111 (QSA + MTP decode graph corruption on GB10 TP2), #36545 (fp8 KV + QSA, SM120).
