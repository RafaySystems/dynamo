"""Static compatibility check: ai-dynamo 1.4.2's dynamo.sglang against the installed SGLang.

Run inside the image (no GPU needed):  python3 compat.py
"""
import argparse
import ast
import importlib
import importlib.metadata as md
import pkgutil
from pathlib import Path

for pkg in ("sglang", "sgl-kernel", "ai-dynamo", "ai-dynamo-runtime", "nixl", "nixl-cu13", "mooncake-transfer-engine",
            "torch", "flashinfer-python", "transformers"):
    try:
        print(f"{pkg:26} {md.version(pkg)}")
    except md.PackageNotFoundError:
        print(f"{pkg:26} -")

import dynamo.sglang  # noqa: E402

root = Path(dynamo.sglang.__file__).parent

# 1. Every dynamo.sglang submodule imports.
print("\n== dynamo.sglang submodule imports")
fails = []
for m in pkgutil.walk_packages([str(root)], prefix="dynamo.sglang."):
    if ".tests" in m.name:
        continue
    try:
        importlib.import_module(m.name)
    except Exception as e:  # noqa: BLE001
        fails.append((m.name, f"{type(e).__name__}: {str(e)[:160]}"))
print(f"{len(fails)} failed" + "".join(f"\n  {n}: {e}" for n, e in fails))

# 2. Every `from sglang... import X` used by dynamo.sglang resolves (also in lazily imported code).
print("\n== sglang symbols referenced by dynamo.sglang")
missing = []
refs = 0
for py in root.rglob("*.py"):
    if "tests" in py.parts:
        continue
    for node in ast.walk(ast.parse(py.read_text())):
        if isinstance(node, ast.ImportFrom) and node.module and node.module.startswith("sglang"):
            for alias in node.names:
                refs += 1
                try:
                    mod = importlib.import_module(node.module)
                    if alias.name != "*" and not hasattr(mod, alias.name):
                        importlib.import_module(f"{node.module}.{alias.name}")
                except Exception as e:  # noqa: BLE001
                    missing.append(f"{py.relative_to(root)}:{node.lineno} from {node.module} import {alias.name} "
                                   f"({type(e).__name__})")
print(f"{refs} references, {len(missing)} unresolved" + "".join(f"\n  {m}" for m in missing))

# 3. The flags the gaap controller injects parse with this SGLang's ServerArgs.
print("\n== gaap-injected SGLang flags")
from sglang.srt.server_args import ServerArgs  # noqa: E402

parser = argparse.ArgumentParser()
ServerArgs.add_cli_args(parser)
gaap = ["--model-path", "/models/./", "--served-model-name", "qwen38-flash-next-pd", "--page-size", "16",
        "--tp-size", "4", "--disaggregation-mode", "decode", "--disaggregation-transfer-backend", "nixl",
        "--disaggregation-bootstrap-port", "12345",
        "--kv-events-config", '{"publisher":"zmq","topic":"kv-events","endpoint":"tcp://*:5557"}']
ns, unknown = parser.parse_known_args(gaap)
print("unknown flags:", unknown or "none",
      f"| transfer_backend={ns.disaggregation_transfer_backend} mode={ns.disaggregation_mode}")

# 4. Model support present in this SGLang.
print("\n== model support")
import sglang  # noqa: E402

sg = Path(sglang.__file__).parent
print("qwen4_exp model file:", (sg / "srt/models/qwen4_exp.py").exists())
dis = "".join(p.read_text(errors="ignore") for p in (sg / "srt/disaggregation").rglob("*.py"))
print("PD state transfer mentions Qwen4Exp/PLE:", any(k in dis for k in ("qwen4_exp", "Qwen4Exp", "ple_", "PLE")))
