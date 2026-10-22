# Extends the release frontend image with the repo's chat templates baked in
# at the image root, so relative paths like
# `examples/chat_templates/gemma4_tool.jinja` resolve at runtime.
#
# Build from the repo root (so the examples/ path is in the build context):
#   docker build -f hack/Dockerfile -t dynamo-frontend:1.2.1-chat-templates .
FROM nvcr.io/nvidia/ai-dynamo/dynamo-frontend:1.2.1

# Deterministic hashing for reproducible behavior across frontend processes.
ENV PYTHONHASHSEED=0

COPY examples/chat_templates /examples/chat_templates
