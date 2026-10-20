# syntax=docker/dockerfile:1.10.0-labs
# SPDX-FileCopyrightText: Copyright (c) 2024-2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
# dynamo-frontend 1.4.2 + Rafay fixes + frontend-crates#408 parser fix, as one self-contained
# multi-stage build (no prebuilt intermediate images).
#
#   docker build --platform linux/amd64 \
#     -f hack/dynamo-frontend-parserfix/Dockerfile \
#     --build-arg EPP_GIT_COMMIT=$(git rev-parse HEAD) \
#     -t registry.dev.rafay-edge.net/tf/dynamo-frontend:1.4.2-fixes-parserfix .
#
# Build context is the REPOSITORY ROOT of RafaySystems/dynamo branch 1.4.2-fixes-parserfix.
# The parser fix itself is not in this file: it comes from the [patch.crates-io] entries in
# Cargo.toml and lib/bindings/python/Cargo.toml (dynamo-parsers + dynamo-protocols pinned to
# RafaySystems/frontend-crates@2062845874), which both the Python wheel and the EPP compile.
#
# Generated from `container/render.py --framework=dynamo --target=frontend` on this branch, then:
#   * `FROM ${EPP_IMAGE} AS epp` replaced by the epp_rust_builder / epp_go_builder / epp stages
#     below (from hack/Dockerfile.epp + the SBOM steps of deploy/inference-gateway/epp/Dockerfile),
#     so the Dynamo EPP with the same fix is compiled in-file;
#   * the licenses stage reads the EPP Go SBOM from epp_go_builder instead of a .epp-sbom/ dir
#     in the build context.
# See hack/dynamo-frontend-parserfix/README.md.

# === BEGIN templates/args.Dockerfile ===
##########################
#### Build Arguments #####
##########################
# TARGETARCH is set automatically by Docker BuildKit for every --platform build.
# It must NOT be declared in the global scope (before any FROM) — doing so shadows
# the automatic per-platform value that BuildKit injects.
#
# In each stage that needs it, re-declare with:  ARG TARGETARCH
#
# ARCH_ALT (x86_64 / aarch64) is computed inline in RUN steps:
#   ARCH_ALT=$([ "${TARGETARCH}" = "amd64" ] && echo "x86_64" || echo "aarch64")
ARG DEVICE=cuda

# Python/CUDA configuration
ARG PYTHON_VERSION=3.12
ARG CUDA_VERSION=13.0
ARG CUDA_MAJOR=${CUDA_VERSION%%.*}

# Base and runtime images configuration
ARG BASE_IMAGE=nvcr.io/nvidia/cuda-dl-base
ARG BASE_IMAGE_TAG=25.11-cuda13.0-devel-ubuntu24.04

# wheel builder image selection

ARG WHEEL_BUILDER_IMAGE=quay.io/pypa/manylinux_2_28_x86_64

# Build configuration
ARG ENABLE_KVBM=true
ARG CARGO_BUILD_JOBS

ARG NATS_VERSION=v2.12.14
ARG ETCD_VERSION=v3.5.33

ARG ENABLE_MEDIA_FFMPEG=false
ARG FFMPEG_VERSION=8.1.2
ARG LIBVPX_REF=v1.14.1
ARG ENABLE_GPU_MEMORY_SERVICE=true

# SCCACHE configuration
ARG USE_SCCACHE
ARG SCCACHE_VERSION=v0.14.0
ARG SCCACHE_BUCKET=""
ARG SCCACHE_REGION=""

# NIXL configuration
ARG NIXL_UCX_REF=v1.21.0

ARG NIXL_REF=v1.3.2

ARG NIXL_GDRCOPY_REF=v2.5.2
ARG NIXL_LIBFABRIC_REPO=https://github.com/ofiwg/libfabric.git
ARG NIXL_LIBFABRIC_REF=v2.4.0
ARG HWLOC_VERSION=2.12.2

# EPP toolchains (the EPP is compiled in-file; see the epp_* stages).
ARG EPP_RUST_IMAGE=rust:1.96.1
ARG EPP_GO_IMAGE=golang:1.26.6
ARG FRONTEND_IMAGE=nvcr.io/nvidia/base/ubuntu:noble-20250619

# --- Base Image Stages

# === BEGIN templates/dynamo_base.Dockerfile ===
##################################
########## Base Image ############
##################################

FROM ${BASE_IMAGE}:${BASE_IMAGE_TAG} AS dynamo_base

ARG TARGETARCH

USER root
WORKDIR /opt/dynamo

# Install sccache into the base image so downstream stages can COPY it
# instead of downloading from GitHub (avoids 502 errors under parallel builds)
ARG SCCACHE_VERSION
RUN ARCH_ALT=$([ "${TARGETARCH}" = "amd64" ] && echo "x86_64" || echo "aarch64") && \
    wget --tries=3 --waitretry=5 \
        "https://github.com/mozilla/sccache/releases/download/${SCCACHE_VERSION}/sccache-${SCCACHE_VERSION}-${ARCH_ALT}-unknown-linux-musl.tar.gz" && \
    tar -xzf "sccache-${SCCACHE_VERSION}-${ARCH_ALT}-unknown-linux-musl.tar.gz" && \
    mv "sccache-${SCCACHE_VERSION}-${ARCH_ALT}-unknown-linux-musl/sccache" /usr/local/bin/ && \
    rm -rf sccache*

# Install uv package manager. It lives in a directory of its own, prepended to
# PATH, because some bases ship a uv earlier on PATH than /usr/local/bin
# (/root/.local/bin, /opt/venv/bin). All stages share one uv cache mount and uv
# rejects cache entries written by a newer version, so the pinned copy has to be
# the one that runs. Holding only uv/uvx keeps the prepend from shadowing
# anything else, notably a framework venv's python.
COPY --from=ghcr.io/astral-sh/uv:0.12.0 /uv /uvx /opt/uv/bin/
ENV PATH=/opt/uv/bin:${PATH}

# Install NATS server
ARG NATS_VERSION
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    wget --tries=3 --waitretry=5 https://github.com/nats-io/nats-server/releases/download/${NATS_VERSION}/nats-server-${NATS_VERSION}-${TARGETARCH}.deb && \
    dpkg -i nats-server-${NATS_VERSION}-${TARGETARCH}.deb && rm nats-server-${NATS_VERSION}-${TARGETARCH}.deb

# Install etcd
ARG ETCD_VERSION
RUN wget --tries=3 --waitretry=5 https://github.com/etcd-io/etcd/releases/download/$ETCD_VERSION/etcd-$ETCD_VERSION-linux-${TARGETARCH}.tar.gz -O /tmp/etcd.tar.gz && \
    mkdir -p /usr/local/bin/etcd && \
    tar -xvf /tmp/etcd.tar.gz -C /usr/local/bin/etcd --strip-components=1 && \
    rm /tmp/etcd.tar.gz
ENV PATH=/usr/local/bin/etcd/:$PATH

# Rust Setup
ENV RUSTUP_HOME=/usr/local/rustup \
    CARGO_HOME=/usr/local/cargo \
    PATH=/usr/local/cargo/bin:$PATH \
    RUST_VERSION=1.96.1

# Install Rust — ARCH_ALT (x86_64/aarch64) is derived from TARGETARCH at build time
RUN ARCH_ALT=$([ "${TARGETARCH}" = "amd64" ] && echo "x86_64" || echo "aarch64") && \
    RUSTARCH="${ARCH_ALT}-unknown-linux-gnu" && \
    wget --tries=3 --waitretry=5 "https://static.rust-lang.org/rustup/archive/1.28.1/${RUSTARCH}/rustup-init" && \
    chmod +x rustup-init && \
    ./rustup-init -y --no-modify-path --profile minimal --default-toolchain $RUST_VERSION --default-host ${RUSTARCH} && \
    rm rustup-init && \
    chmod -R a+w $RUSTUP_HOME $CARGO_HOME

# === BEGIN templates/wheel_builder.Dockerfile ===
##################################
##### Wheel Build Image ##########
##################################

##################################
##### wheel_builder_base #########
##################################
# Shared base for all wheel builds: tools, system deps, and native libraries (except nixl).

FROM ${WHEEL_BUILDER_IMAGE} AS wheel_builder_base

# Redeclare ARGs for this stage
ARG TARGETARCH
ARG CARGO_BUILD_JOBS
ARG DEVICE

WORKDIR /workspace

# Compliance: always create the rust license-harvest dir so the licenses stage's
# `COPY --from=wheel_builder /opt/dynamo/rust-licenses` never fails, even for
# targets that build no wheels. runtime_wheel_builder populates it post-build.
RUN mkdir -p /opt/dynamo/rust-licenses

# Copy CUDA from base stage
COPY --from=dynamo_base /usr/local/cuda /usr/local/cuda
COPY --from=dynamo_base /etc/ld.so.conf.d/hpcx.conf /etc/ld.so.conf.d/hpcx.conf

# Set environment variables first so they can be used in COPY commands
ENV CARGO_BUILD_JOBS=${CARGO_BUILD_JOBS:-16} \
    RUSTUP_HOME=/usr/local/rustup \
    CARGO_HOME=/usr/local/cargo \
    CARGO_TARGET_DIR=/opt/dynamo/target \
    PATH=/usr/local/cargo/bin:$PATH

# Copy artifacts from base stage
COPY --from=dynamo_base $RUSTUP_HOME $RUSTUP_HOME
COPY --from=dynamo_base $CARGO_HOME $CARGO_HOME

# Install system dependencies
# Cache dnf downloads; sharing=locked avoids dnf/rpm races with concurrent builds.
# --setopt=tsflags=nocontexts: skip SELinux file-context labeling. The manylinux
# image lacks the SELinux policy store that some compute nodes expect; without
# this flag, dnf fails with "ValueError: SELinux policy is not managed".
RUN --mount=type=cache,target=/var/cache/dnf,sharing=locked \
    dnf install -y --setopt=tsflags=nocontexts almalinux-release-synergy && \
    dnf config-manager --set-enabled powertools && \
    dnf install -y --setopt=tsflags=nocontexts \
        # Autotools (required for UCX, libfabric ./autogen.sh and ./configure)
        autoconf \
        automake \
        libtool \
        make \
        # RPM build tools (required for gdrcopy's build-rpm-packages.sh)
        rpm-build \
        rpm-sign \
        # Build tools
        cmake \
        ninja-build \
        clang-devel \
        # Install GCC toolset 14 (CUDA compatible, max version 14)
        gcc-toolset-14-gcc \
        gcc-toolset-14-gcc-c++ \
        gcc-toolset-14-binutils \
        flex \
        wget \
        # Kernel module build dependencies
        dkms \
        # Protobuf support
        protobuf-compiler \
        # RDMA/InfiniBand support (required for UCX build with --with-verbs)
        libibverbs \
        libibverbs-devel \
        rdma-core \
        rdma-core-devel \
        libibumad \
        libibumad-devel \
        librdmacm-devel \
        numactl-devel \
        # Libfabric support
        libcurl-devel \
        openssl-devel \
        libuuid-devel \
        zlib-devel

# Default comes from context.yaml; keep it in sync with upstream NIXL's
# contrib/Dockerfile.manylinux. NIXL v1.0.x needs newer hwloc than RHEL8 ships.
ARG HWLOC_VERSION
RUN cd /tmp && \
    HWLOC_SERIES="$(echo "${HWLOC_VERSION}" | cut -d. -f1,2)" && \
    wget -q "https://download.open-mpi.org/release/hwloc/v${HWLOC_SERIES}/hwloc-${HWLOC_VERSION}.tar.gz" && \
    tar -xzf "hwloc-${HWLOC_VERSION}.tar.gz" && \
    cd "hwloc-${HWLOC_VERSION}" && \
    ./configure --prefix=/usr/local --disable-nvml && \
    make -j"$(nproc)" && \
    make install && \
    ldconfig && \
    rm -rf "/tmp/hwloc-${HWLOC_VERSION}" "/tmp/hwloc-${HWLOC_VERSION}.tar.gz"

# Set GCC toolset 14 as the default compiler (CUDA requires GCC <= 14)
ENV PATH="/opt/rh/gcc-toolset-14/root/usr/bin:${PATH}" \
    LD_LIBRARY_PATH="/opt/rh/gcc-toolset-14/root/usr/lib64:${LD_LIBRARY_PATH}" \
    CC="/opt/rh/gcc-toolset-14/root/usr/bin/gcc" \
    CXX="/opt/rh/gcc-toolset-14/root/usr/bin/g++"

# Ensure a modern protoc is available (required for --experimental_allow_proto3_optional)
RUN set -eux; \
    ARCH_ALT=$([ "${TARGETARCH}" = "amd64" ] && echo "x86_64" || echo "aarch64"); \
    PROTOC_VERSION=25.3; \
    case "${ARCH_ALT}" in \
      x86_64) PROTOC_ZIP="protoc-${PROTOC_VERSION}-linux-x86_64.zip" ;; \
      aarch64) PROTOC_ZIP="protoc-${PROTOC_VERSION}-linux-aarch_64.zip" ;; \
      *) echo "Unsupported architecture: ${ARCH_ALT}" >&2; exit 1 ;; \
    esac; \
    wget --tries=3 --waitretry=5 -O /tmp/protoc.zip "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/${PROTOC_ZIP}"; \
    rm -f /usr/local/bin/protoc /usr/bin/protoc; \
    unzip -o /tmp/protoc.zip -d /usr/local bin/protoc include/*; \
    chmod +x /usr/local/bin/protoc; \
    ln -s /usr/local/bin/protoc /usr/bin/protoc; \
    protoc --version

# Point build tools explicitly at the modern protoc
ENV PROTOC=/usr/local/bin/protoc

# Install uv package manager, ahead of the copy manylinux bundles in
# /usr/local/bin. See dynamo_base.Dockerfile for why it gets its own directory.
COPY --from=ghcr.io/astral-sh/uv:0.12.0 /uv /uvx /opt/uv/bin/
ENV PATH=/opt/uv/bin:${PATH}

ENV CUDA_PATH=/usr/local/cuda \
    PATH=/usr/local/cuda/bin:$PATH \
    LD_LIBRARY_PATH=/usr/local/cuda/lib64:/usr/local/lib:/usr/local/lib64:${LD_LIBRARY_PATH:-} \
    NVIDIA_DRIVER_CAPABILITIES=video,compute,utility

# Create virtual environment for building wheels
ARG PYTHON_VERSION
ENV VIRTUAL_ENV=/workspace/.venv
# Cache uv downloads; uv handles its own locking for this cache.
# pyyaml: needed by the compliance NOTICES-bundling steps below (overrides.py
# imports yaml at module scope); the system python3 doesn't ship it.
RUN --mount=type=cache,id=uv-root-0.12.0,target=/root/.cache/uv,sharing=shared \
    export UV_CACHE_DIR=/root/.cache/uv UV_HTTP_TIMEOUT=300 UV_HTTP_RETRIES=5 && \
    uv venv ${VIRTUAL_ENV} --python $PYTHON_VERSION && \
    uv pip install --upgrade meson pybind11 patchelf maturin[patchelf] tomlkit pyyaml

ARG NIXL_UCX_REF

ARG NIXL_GDRCOPY_REF

# Build and install gdrcopy
RUN ARCH_ALT=$([ "${TARGETARCH}" = "amd64" ] && echo "x86_64" || echo "aarch64") && \
    git clone --depth 1 --branch ${NIXL_GDRCOPY_REF} https://github.com/NVIDIA/gdrcopy.git && \
    cd gdrcopy/packages && \
    CUDA=/usr/local/cuda ./build-rpm-packages.sh && \
    rpm -Uvh gdrcopy-kmod-*.el8.noarch.rpm && \
    rpm -Uvh gdrcopy-*.el8.${ARCH_ALT}.rpm && \
    rpm -Uvh gdrcopy-devel-*.el8.noarch.rpm

# sccache binary is pre-installed in dynamo_base; stage it off-PATH so
# Meson doesn't auto-detect it as a CUDA compiler launcher
# (https://github.com/mesonbuild/meson/issues/11118).
# When USE_SCCACHE=true the RUN below symlinks it onto PATH before install.
COPY --from=dynamo_base /usr/local/bin/sccache /opt/sccache/sccache

ARG USE_SCCACHE
ARG SCCACHE_BUCKET
ARG SCCACHE_REGION
COPY container/use-sccache.sh /tmp/use-sccache.sh
RUN if [ "$USE_SCCACHE" = "true" ]; then \
        ln -s /opt/sccache/sccache /usr/local/bin/sccache && \
        /tmp/use-sccache.sh install; \
    fi

# Compliance: native source archives drop here. RUN git clone / wget …tar lines
# in the wheel_builder pipeline preserve their resulting archive at
# /tmp/native-sources/<name>-<version>.tar.gz so the per-image `sources_collect`
# stage can COPY them out for OSRB submission. Created here unconditionally
# (cheap) so the COPY always succeeds even when no native source builds run
# for this framework.
RUN mkdir -p /tmp/native-sources

# Compliance source-archival pattern (do NOT add ARG ENABLE_SOURCE_ARCHIVAL
# at this scope — it would invalidate every downstream layer when the flag
# flips between PR builds and post-merge builds).
#
# When future work adds cargo-vendor / go-mod-vendor / native source-tree
# preservation, declare the ARG INLINE in the smallest possible scope,
# immediately before the gated RUN, e.g.:
#
#     ARG ENABLE_SOURCE_ARCHIVAL=false
#     RUN if [ "$ENABLE_SOURCE_ARCHIVAL" = "true" ]; then \
#           cargo vendor --locked --manifest-path /opt/dynamo/Cargo.toml \
#               /tmp/native-sources/rust-vendor; \
#         fi
#
# This way the cache invalidation is contained to one RUN layer (the gated
# one), not the rest of wheel_builder_base. shared-build-image.yml passes
# ENABLE_SOURCE_ARCHIVAL=true via extra_build_args on push / release /
# workflow_dispatch events; PR builds get the default "false" and skip.

# Set SCCACHE environment variables (RUSTC_WRAPPER is set dynamically by
# setup-env only when the sccache server starts successfully)
ENV SCCACHE_BUCKET=${USE_SCCACHE:+${SCCACHE_BUCKET}} \
    SCCACHE_REGION=${USE_SCCACHE:+${SCCACHE_REGION}}

# Build FFmpeg for every framework's video-encode path, SGLang included. The
# build is VP9-only (libvpx) — it contains no H.264, H.265, or AAC encoder in
# any form — so SGLang's video-generation handler gets a VP9 encoder to write
# with, matching vLLM/TRT-LLM.
# Build FFmpeg so libs are available for Rust checks in CI.
# We build the ffmpeg CLI with the libvpx_vp9 encoder so Python code can encode
# video without the GPL-licensed binary shipped by imageio-ffmpeg.
# Stays LGPL-only AND royalty-free: --disable-gpl --disable-nonfree are preserved,
# and no H.264/H.265/AAC codec is built in any form. Video encode is VP9 only
# (libvpx, BSD). NVENC is intentionally NOT enabled: the hardware H.264 encoder is
# still a distributable H.264 codec surface, so it is omitted entirely — see the
# post-build guard below that fails the build if any H.264 surface reappears.
#
# MEDIA CODEC ALLOWLIST: the in-tree libavcodec should carry only
# the media formats we actually build and use, not ffmpeg's full default decoder
# set. A blanket --disable-decoders/--disable-demuxers/--disable-parsers plus a
# narrow allowlist keeps the shipped libav*.so limited to that set. The allowlist
# covers exactly two paths: (1) the encode CLI ingesting rawvideo frames from
# imageio over a pipe and encoding with libvpx_vp9, and (2) the Rust media-ffmpeg
# VideoDecoder decoding VP8/VP9 in mp4/webm/mkv (test fixtures are VP9-in-mp4).
# No H.264 parser/decoder/encoder is enabled — H.264 is not built at all. Image
# decode does not use ffmpeg (it goes through the Rust `image` crate), so no
# still-image decoders are enabled here.
# The `fd` protocol is enabled alongside `pipe`: `ffmpeg -i -` reads stdin via
# the `fd:` protocol on ffmpeg 8.x (not `pipe:`), so omitting it breaks the
# imageio encode path with "Protocol not found. Did you mean file:fd:?". Both
# are pure fd/stream I/O and carry no codec implementation.
#
# Combined with the 8.1 -> 8.1.2 bump below (an upstream maintenance release),
# this also trims the decoder surface to what we ship.
# Do not delete the source tarball for legal reasons.
ARG FFMPEG_VERSION
ARG LIBVPX_REF
RUN --mount=type=secret,id=aws-web-identity-token,target=/run/secrets/aws-token \
    --mount=type=secret,id=aws-role-arn,env=AWS_ROLE_ARN \
    export AWS_WEB_IDENTITY_TOKEN_FILE=/run/secrets/aws-token && \
    export SCCACHE_S3_KEY_PREFIX=${SCCACHE_S3_KEY_PREFIX:-${TARGETARCH}} && \
    if [ "$USE_SCCACHE" = "true" ]; then \
        eval $(/tmp/use-sccache.sh setup-env); \
    fi && \
    if [ "$DEVICE" = "xpu" ] || [ "$DEVICE" = "cpu" ]; then \
    apt-get update -y && apt-get install -y build-essential pkg-config xz-utils git yasm; \
    apt-get clean && rm -rf /var/lib/apt/lists/*; \
    elif [ "$DEVICE" = "cuda" ]; then \
    dnf install -y --setopt=tsflags=nocontexts pkg-config xz git yasm; \
    fi && \
    # No nv-codec-headers: NVENC/NVDEC are not built, so the NVIDIA codec API
    # headers are not needed. This keeps H.264 (incl. the h264_nvenc HW encoder)
    # out of the in-tree ffmpeg entirely.
    cd /tmp && \
    # libvpx: BSD-licensed VP9 encoder needed for the WebM output path. Built from
    # source so we don't need to track distro package names (libvpx-dev on Debian
    # vs libvpx-devel via EPEL on RHEL/manylinux).
    git clone --depth 1 --branch ${LIBVPX_REF} https://chromium.googlesource.com/webm/libvpx.git && \
    cd libvpx && \
    ./configure --prefix=/usr/local --enable-shared --disable-static --disable-examples --disable-unit-tests --disable-tools --disable-docs && \
    make -j$(nproc) && \
    make install && \
    ldconfig && \
    cd /tmp && \
    # Retry the ffmpeg fetch in a shell loop: curl's own --retry does not cover
    # SSL/connection-level failures (e.g. `curl: (35) SSL_ERROR_SYSCALL`) unless
    # --retry-all-errors is used, which needs curl >= 7.71 (the manylinux build
    # base ships 7.61). The loop retries on ANY failure and is version-agnostic.
    for attempt in 1 2 3 4 5; do \
        curl --retry 3 --retry-delay 5 --retry-connrefused --connect-timeout 30 -fL \
            -o ffmpeg-${FFMPEG_VERSION}.tar.xz \
            https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz && break; \
        echo "ffmpeg download attempt ${attempt}/5 failed; retrying in 10s" >&2; \
        sleep 10; \
    done && \
    test -s ffmpeg-${FFMPEG_VERSION}.tar.xz && \
    tar xf ffmpeg-${FFMPEG_VERSION}.tar.xz && \
    cd ffmpeg-${FFMPEG_VERSION} && \
    ./configure \
        --prefix=/usr/local \
        --disable-gpl \
        --disable-nonfree \
        --disable-doc \
        --disable-static \
        --disable-x86asm \
        --disable-network \
        --disable-bsfs \
        --disable-devices \
        --disable-libdrm \
        --enable-shared \
        --enable-libvpx \
        --disable-encoders \
        --enable-encoder=libvpx_vp9 \
        --disable-decoders \
        --enable-decoder=vp8,vp9,rawvideo \
        --disable-muxers \
        --enable-muxer=mov,mp4,matroska,webm \
        --disable-demuxers \
        --enable-demuxer=mov,matroska,rawvideo \
        --disable-parsers \
        --enable-parser=vp8,vp9 \
        --disable-protocols \
        --enable-protocol=file,pipe,fd && \
    make -j$(nproc) && \
    make install && \
    # Compliance guard: fail the build if any royalty-bearing / HW codec surface
    # leaked into the in-tree ffmpeg. By construction this build is VP9-only, so a
    # match here means a config regression. Check the implementation-carrying
    # surfaces (encoders/decoders/parsers), not -codecs (lists names even when no
    # implementation is built) and not -bsfs: bitstream filters (e.g.
    # aac_adtstoasc, h264_mp4toannexb) only reframe an already-encoded stream, are
    # pulled in as mov/mp4 muxer dependencies, and carry no codec implementation.
    for surface in encoders decoders parsers; do \
        if /usr/local/bin/ffmpeg -hide_banner "-${surface}" 2>/dev/null \
             | grep -qiE 'h\.?264|h\.?265|hevc|(^| )aac|nvenc|cuvid|nvdec'; then \
            echo "ERROR: in-tree ffmpeg exposes a disallowed codec via -${surface}" >&2; \
            /usr/local/bin/ffmpeg -hide_banner "-${surface}" 2>/dev/null \
             | grep -iE 'h\.?264|h\.?265|hevc|(^| )aac|nvenc|cuvid|nvdec' >&2; \
            exit 1; \
        fi; \
    done && \
    /tmp/use-sccache.sh show-stats "FFMPEG" && \
    ldconfig && \
    mkdir -p /usr/local/src/ffmpeg && \
    find /tmp/ffmpeg-${FFMPEG_VERSION} \( -name config.log -o -name config.status \) -delete && \
    mv /tmp/ffmpeg-${FFMPEG_VERSION}* /usr/local/src/ffmpeg/

# Build and install UCX
RUN --mount=type=secret,id=aws-web-identity-token,target=/run/secrets/aws-token \
    --mount=type=secret,id=aws-role-arn,env=AWS_ROLE_ARN \
    export AWS_WEB_IDENTITY_TOKEN_FILE=/run/secrets/aws-token && \
    export SCCACHE_S3_KEY_PREFIX="${SCCACHE_S3_KEY_PREFIX:-${TARGETARCH}}" && \
    if [ "$USE_SCCACHE" = "true" ]; then \
        eval $(/tmp/use-sccache.sh setup-env); \
    fi && \
    cd /usr/local/src && \
    git clone https://github.com/openucx/ucx.git && \
    cd ucx &&  \
    git checkout $NIXL_UCX_REF &&	 \
    # The intel/llm-scaler xe-GDR patch (ucx-v1.12.0.patch) is upstream since
    # UCX v1.21.0 (ib_md.c xe srcversion check, ze_copy_md.c HOST bit); restore
    # the fetch + git apply for DEVICE=xpu if this ref ever drops below v1.21.0.
    ./autogen.sh &&      \
    if [ "$DEVICE" = "xpu" ]; then \
     ./contrib/configure-release     \
        --prefix=/usr/local/ucx     \
        --with-ze                   \
        --enable-shared             \
        --disable-static            \
        --disable-doxygen-doc       \
        --enable-optimizations      \
        --enable-cma                \
        --enable-devel-headers      \
        --with-verbs                \
        --with-dm                   \
        --with-efa                  \
        --without-cuda              \
        --enable-mt;                 \
    elif [ "$DEVICE" = "cuda" ]; then \
     ./contrib/configure-release     \
        --prefix=/usr/local/ucx     \
        --enable-shared             \
        --disable-static            \
        --disable-doxygen-doc       \
        --enable-optimizations      \
        --enable-cma                \
        --enable-devel-headers      \
        --with-cuda=/usr/local/cuda \
        --with-verbs                \
        --with-dm                   \
        --with-gdrcopy=/usr/local   \
        --with-efa                  \
        --enable-mt;                 \
    elif [ "$DEVICE" = "cpu" ]; then  \
     ./contrib/configure-release     \
        --prefix=/usr/local/ucx     \
        --enable-shared             \
        --disable-static            \
        --disable-doxygen-doc       \
        --enable-optimizations      \
        --enable-cma                \
        --enable-devel-headers      \
        --with-verbs                \
        --without-cuda              \
        --enable-mt;                 \
     fi && \
     make -j &&                      \
     make -j install-strip &&        \
     /tmp/use-sccache.sh show-stats "UCX" && \
     echo "/usr/local/ucx/lib" > /etc/ld.so.conf.d/ucx.conf && \
     echo "/usr/local/ucx/lib/ucx" >> /etc/ld.so.conf.d/ucx.conf && \
     ldconfig

ARG NIXL_LIBFABRIC_REPO
ARG NIXL_LIBFABRIC_REF
RUN --mount=type=secret,id=aws-web-identity-token,target=/run/secrets/aws-token \
    --mount=type=secret,id=aws-role-arn,env=AWS_ROLE_ARN \
    export AWS_WEB_IDENTITY_TOKEN_FILE=/run/secrets/aws-token && \
    export SCCACHE_S3_KEY_PREFIX="${SCCACHE_S3_KEY_PREFIX:-${TARGETARCH}}" && \
    if [ "$USE_SCCACHE" = "true" ]; then \
        eval $(/tmp/use-sccache.sh setup-env); \
    fi && \
    cd /usr/local/src && \
    git clone "${NIXL_LIBFABRIC_REPO}" && \
    cd libfabric && \
    git checkout $NIXL_LIBFABRIC_REF && \
    ./autogen.sh && \
    ./configure --prefix="/usr/local/libfabric" \
                --disable-verbs \
                --disable-psm3 \
                --disable-opx \
                --disable-usnic \
                --disable-rstream \
                --enable-efa \
                --with-cuda=/usr/local/cuda \
                --enable-cuda-dlopen \
                --with-gdrcopy \
                --enable-gdrcopy-dlopen && \
    make -j$(nproc) && \
    make install && \
    /tmp/use-sccache.sh show-stats "LIBFABRIC" && \
    echo "/usr/local/libfabric/lib" > /etc/ld.so.conf.d/libfabric.conf && \
    ldconfig

ENV PKG_CONFIG_PATH="/usr/local/libfabric/lib/pkgconfig:${PKG_CONFIG_PATH}"

##################################
##### runtime_wheel_builder ######
##################################
# Builds ai-dynamo, ai-dynamo-runtime, and gpu_memory_service wheels, sans nixl.

FROM wheel_builder_base AS runtime_wheel_builder

# Copy source code (order matters for layer caching)
COPY .cargo/ /opt/dynamo/.cargo/
COPY pyproject.toml README.md LICENSE Cargo.toml Cargo.lock rust-toolchain.toml hatch_build.py /opt/dynamo/
COPY lib/ /opt/dynamo/lib/
COPY components/ /opt/dynamo/components/

# Build ai-dynamo (pure Python) and ai-dynamo-runtime (maturin) wheels
ARG USE_SCCACHE

ARG ENABLE_MEDIA_FFMPEG

RUN --mount=type=secret,id=aws-web-identity-token,target=/run/secrets/aws-token \
    --mount=type=secret,id=aws-role-arn,env=AWS_ROLE_ARN \
    --mount=type=cache,target=/root/.cargo/registry,sharing=shared \
    --mount=type=cache,target=/root/.cargo/git,sharing=shared \
    --mount=type=cache,id=uv-root-0.12.0,target=/root/.cache/uv,sharing=shared \
    export AWS_WEB_IDENTITY_TOKEN_FILE=/run/secrets/aws-token && \
    export UV_CACHE_DIR=/root/.cache/uv && \
    export SCCACHE_S3_KEY_PREFIX=${SCCACHE_S3_KEY_PREFIX:-${TARGETARCH}} && \
    if [ "$USE_SCCACHE" = "true" ]; then \
        eval $(/tmp/use-sccache.sh setup-env cmake); \
    fi && \
    mkdir -p ${CARGO_TARGET_DIR} && \
    source ${VIRTUAL_ENV}/bin/activate && \
    cd /opt/dynamo && \
    uv build --wheel --out-dir /opt/dynamo/dist && \
    cd /opt/dynamo/lib/bindings/python && \
    if [ "$ENABLE_MEDIA_FFMPEG" = "true" ]; then \
        maturin build --release --features "media-ffmpeg,kv-indexer,slot-tracker,select-service,mm-routing,aic-forward-pass" --out /opt/dynamo/dist; \
    else \
        maturin build --release --features "kv-indexer,slot-tracker,select-service,mm-routing,aic-forward-pass" --out /opt/dynamo/dist; \
    fi && \
    /tmp/use-sccache.sh show-stats "Dynamo Runtime"

# Compliance: harvest each crate's real LICENSE files from the cargo registry
# source cache so the rust NOTICES generator can inline upstream license text
# (the runtime image keeps only the compiled wheel). Keyed "<name>-<version>"
# to match generators/rust.py. Best-effort: unreadable/absent files are skipped
# and the generator falls back to canonical SPDX text. cargo's registry lives
# under CARGO_HOME and/or the cache-mounted /root/.cargo — scan both.
RUN --mount=type=cache,target=/root/.cargo/registry,sharing=shared \
    for src in "${CARGO_HOME}/registry/src" /root/.cargo/registry/src; do \
        [ -d "$src" ] || continue; \
        find "$src" -mindepth 2 -maxdepth 2 -type d | while IFS= read -r crate; do \
            dest="/opt/dynamo/rust-licenses/$(basename "$crate")"; \
            for lf in "$crate"/LICENSE* "$crate"/LICENCE* "$crate"/COPYING* "$crate"/NOTICE* "$crate"/UNLICENSE*; do \
                [ -e "$lf" ] || continue; \
                mkdir -p "$dest" && cp "$lf" "$dest/" 2>/dev/null || true; \
            done; \
        done; \
    done; \
    echo "rust license harvest: $(find /opt/dynamo/rust-licenses -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l) crates with license files"; \
    true

# Compliance: bundle the human-readable third-party Rust NOTICES into the
# maturin wheels themselves (PEP 639 <dist-info>/licenses/), using the harvested
# crate license texts. The wheel already carries maturin's CycloneDX SBOM (the
# machine-readable inventory); this adds the texts the redistributed wheel's
# MIT/BSD/Apache attribution clauses require. Best-effort + non-fatal: a failure
# leaves the wheel with its SBOM intact rather than breaking the build.
# Must run with the build venv's python: bundle_wheel_notices imports
# compliance.overrides, which needs pyyaml (installed in the venv above);
# the bare system python3 lacks it and the step would no-op with a warning.
COPY container/compliance /opt/compliance
RUN set -u; injected=0; \
    for whl in /opt/dynamo/dist/ai_dynamo_runtime*.whl; do \
        [ -e "$whl" ] || continue; \
        PYTHONPATH=/opt ${VIRTUAL_ENV}/bin/python3 -m compliance.bundle_wheel_notices \
            --wheel "$whl" --licenses-dir /opt/dynamo/rust-licenses -v \
            && injected=$((injected+1)) || echo "::warning::wheel NOTICES bundling failed for $whl (SBOM retained)"; \
    done; \
    echo "wheel NOTICES bundled into $injected wheel(s)"

# Compliance source archival: vendor the workspace lockfile for the OSRB
# bundle. Gated on ENABLE_SOURCE_ARCHIVAL so PR builds skip the ~200-400 MB
# vendor pull. The vendor tree is consumed downstream by each runtime
# template's sources_collect stage, which filters against the installed
# wheels' embedded SBOMs to keep only the third-party crates we actually
# ship. Stay scoped to one RUN layer (cache invalidation contained).
ARG ENABLE_SOURCE_ARCHIVAL=false
# Mount cargo registry + git caches so re-runs don't re-download the
# ~750 crates from crates.io every build. `sharing=shared` lets parallel
# builds (e.g. multiple frameworks in CI) read the same cache concurrently.
RUN --mount=type=cache,target=/root/.cargo/registry,sharing=shared \
    --mount=type=cache,target=/root/.cargo/git,sharing=shared \
    if [ "$ENABLE_SOURCE_ARCHIVAL" = "true" ]; then \
        mkdir -p /tmp/dynamo-vendor-full && \
        cd /opt/dynamo && \
        cargo vendor --locked /tmp/dynamo-vendor-full > /dev/null && \
        cp Cargo.toml Cargo.lock /tmp/dynamo-vendor-full/ ; \
    fi

# Build gpu-memory-service wheel → /opt/dynamo/dist/gpu_memory_service*.whl (small C++ extension, fast build -- all targets, all frameworks)

# Build gpu_memory_service wheel (C++ extension only needs Python headers, no CUDA/torch)
ARG ENABLE_GPU_MEMORY_SERVICE
RUN --mount=type=cache,id=uv-root-0.12.0,target=/root/.cache/uv,sharing=shared \
    if [ "$ENABLE_GPU_MEMORY_SERVICE" = "true" ]; then \
        export UV_CACHE_DIR=/root/.cache/uv && \
        source ${VIRTUAL_ENV}/bin/activate && \
        uv build --wheel --out-dir /opt/dynamo/dist /opt/dynamo/lib/gpu_memory_service; \
    fi

##################################
##### wheel_builder ##############
##################################

# SGLang CUDA uses NIXL from the upstream lmsysorg/sglang runtime image and the
# frontend installs it from PyPI; neither builds Dynamo KVBM. Keep this alias so
# downstream stages can still COPY Dynamo wheels and build tools from a common
# wheel_builder stage name.
# SGLang dev/source builds may link nixl-sys against stubs when native NIXL is
# absent; block-manager/KVBM runtime work should use vllm/trtllm/none images.
FROM runtime_wheel_builder AS wheel_builder

# === BEGIN templates/frontend.Dockerfile ===
##############################################
########## Frontend entrypoint image #########
##############################################
FROM ${EPP_RUST_IMAGE} AS epp_rust_builder
# Dynamo FFI static library linked into the Go EPP. Built from the same Cargo.lock as the wheel,
# so it carries the same patched dynamo-parsers.
RUN apt-get update && apt-get install -y --no-install-recommends \
        protobuf-compiler libclang-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /dynamo
COPY .cargo/ .cargo/
COPY Cargo.toml Cargo.lock README.md ./
COPY lib/ lib/
# ext-proc is a workspace member; cargo needs its manifest to resolve the workspace.
COPY deploy/inference-gateway/ext-proc/ deploy/inference-gateway/ext-proc/
RUN cargo build --locked --release -p libdynamo_llm \
    && mkdir -p /out && cp target/release/libdynamo_llm_capi.a /out/ \
    && HEADER=$(find target/release/build -name llm_engine.h -path "*/out/*" | head -1) \
    && [ -n "$HEADER" ] && cp "$HEADER" /out/

FROM ${EPP_GO_IMAGE} AS epp_go_builder
ARG EPP_GIT_REF=1.4.2-fixes-parserfix
ARG EPP_GIT_COMMIT=unknown
RUN apt-get update && apt-get install -y --no-install-recommends \
        gcc g++ libc-dev git \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /workspace
COPY deploy/inference-gateway/epp/go.mod deploy/inference-gateway/epp/go.sum ./
RUN go mod download
COPY deploy/inference-gateway/epp/ .
COPY --from=epp_rust_builder /out/libdynamo_llm_capi.a pkg/plugins/dynamo_kv_scorer/lib/
COPY --from=epp_rust_builder /out/llm_engine.h pkg/plugins/dynamo_kv_scorer/include/
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-X sigs.k8s.io/gateway-api-inference-extension/version.GitVersion=${EPP_GIT_REF} \
              -X sigs.k8s.io/gateway-api-inference-extension/version.GitCommit=${EPP_GIT_COMMIT}" \
    -o epp ./cmd/epp
# Go SBOM + module license texts for the frontend licenses stage.
RUN git init -q . \
    && git -c user.email=ci@example -c user.name=ci add -A \
    && git -c user.email=ci@example -c user.name=ci commit -q --allow-empty -m "sbom build" \
    && git tag "v0.0.0-${EPP_GIT_REF}" \
    && GOBIN=/usr/local/bin go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@v1.7.0 \
    && CGO_ENABLED=1 GOOS=linux GOARCH=amd64 cyclonedx-gomod app -licenses -json \
        -main ./cmd/epp -output /sbom-go.cdx.json .
RUN mkdir -p /sbom-go-licenses && cd /go/pkg/mod && \
    find . -path ./cache -prune -o -type f \
        \( -iname 'LICENSE*' -o -iname 'LICENCE*' -o -iname 'COPYING*' \
           -o -iname 'NOTICE*' -o -iname 'UNLICENSE*' \) -print | \
    while IFS= read -r lf; do \
        rel="${lf#./}"; dest="/sbom-go-licenses/$(dirname "$rel")"; \
        mkdir -p "$dest" && cp "$lf" "$dest/" 2>/dev/null || true; \
    done; \
    echo "go license harvest: $(find /sbom-go-licenses -type f | wc -l) files"

FROM scratch AS epp
COPY --from=epp_go_builder /workspace/epp /epp

# NOTE: EPP's Go compliance SBOM (/sbom-go.cdx.json) + harvested license texts are
# NO LONGER pulled from the EPP image here. compliance.Dockerfile's licenses stage
# (this hack build) reads them from the epp_go_builder stage below; upstream reads .epp-sbom/ from the context, populated by the CI EPP-build
# step's `make sbom-export` while the build cache is warm. This replaced a fragile
# COPY --from that re-pulled the pushed EPP image (whose runtime layer could miss
# the files after a BuildKit cache refresh). Only the /epp binary is taken from
# the EPP image (below).

# Build `crick` as a wheel in an isolated stage so the C toolchain never
# reaches the final frontend image. aiperf 0.10.0 depends on crick==0.0.8,
# which publishes no manylinux aarch64 wheel — without this, arm64 builds
# fall back to sdist and fail in the final stage where gcc is intentionally
# absent. amd64 has a prebuilt manylinux_x86_64 wheel on PyPI, so the build
# is gated on TARGETARCH=arm64; amd64 ships an empty /wheels and uv pulls
# crick straight from PyPI in the final stage. /wheels is created either
# way so the COPY --from=crick_builder in the final stage always succeeds.
FROM ${FRONTEND_IMAGE} AS crick_builder
ARG PYTHON_VERSION
ARG TARGETARCH
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    mkdir -p /wheels \
    && if [ "$TARGETARCH" = "arm64" ]; then \
        apt-get update -y \
        && apt-get install -y --no-install-recommends \
            ca-certificates \
            gcc \
            libc6-dev \
            python${PYTHON_VERSION}-dev \
            python${PYTHON_VERSION}-venv \
        && apt-get clean \
        && rm -rf /var/lib/apt/lists/*; \
    fi
RUN if [ "$TARGETARCH" = "arm64" ]; then \
        python${PYTHON_VERSION} -m venv /tmp/buildenv \
        && /tmp/buildenv/bin/pip install --no-cache-dir --upgrade pip wheel \
        && /tmp/buildenv/bin/pip wheel --no-cache-dir --no-deps crick==0.0.8 -w /wheels; \
    fi

# Build the frontend Python environment separately so AIConfigurator can be
# compiled from its immutable Git revision without shipping Git, Cargo, or a C
# toolchain in the final frontend image.
FROM ${FRONTEND_IMAGE} AS frontend_python_deps

ARG PYTHON_VERSION
ENV RUSTUP_HOME=/usr/local/rustup \
    CARGO_HOME=/usr/local/cargo \
    VIRTUAL_ENV=/opt/dynamo/venv \
    PATH="/opt/dynamo/venv/bin:/opt/uv/bin:/usr/local/cargo/bin:${PATH}"

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    apt-get update -y \
    && apt-get install -y --no-install-recommends \
        build-essential \
        ca-certificates \
        git \
        git-lfs \
        patchelf \
        pkg-config \
        python${PYTHON_VERSION}-dev \
    && git lfs install --system \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

COPY --from=ghcr.io/astral-sh/uv:0.12.0 /uv /uvx /opt/uv/bin/
COPY --from=dynamo_base /usr/local/rustup /usr/local/rustup
COPY --from=dynamo_base /usr/local/cargo /usr/local/cargo
COPY --from=crick_builder /wheels/ /opt/dynamo/wheelhouse/extra/

RUN --mount=type=bind,source=./container/deps/requirements.common.txt,target=/tmp/requirements.common.txt \
    --mount=type=bind,source=./container/deps/requirements.frontend.txt,target=/tmp/requirements.frontend.txt \
    --mount=type=bind,source=./container/deps/overrides.frontend.txt,target=/tmp/overrides.frontend.txt \
    --mount=type=cache,id=uv-dynamo-0.12.0,target=/root/.cache/uv,sharing=shared \
    export UV_CACHE_DIR=/root/.cache/uv UV_GIT_LFS=1 UV_HTTP_TIMEOUT=300 UV_HTTP_RETRIES=5 && \
    uv venv /opt/dynamo/venv --python ${PYTHON_VERSION} && \
    uv pip install \
        --overrides /tmp/overrides.frontend.txt \
        --requirement /tmp/requirements.common.txt \
        --requirement /tmp/requirements.frontend.txt

# benchmarks also declares aiconfigurator-core, so install it while the same
# source-build toolchain is available.
RUN --mount=type=bind,source=./benchmarks,target=/tmp/benchmarks,rw \
    --mount=type=bind,source=./container/deps/overrides.frontend.txt,target=/tmp/overrides.frontend.txt \
    --mount=type=cache,id=uv-dynamo-0.12.0,target=/root/.cache/uv,sharing=shared \
    export UV_CACHE_DIR=/root/.cache/uv UV_FIND_LINKS=/opt/dynamo/wheelhouse/extra \
        UV_GIT_LFS=1 UV_HTTP_TIMEOUT=300 UV_HTTP_RETRIES=5 && \
    uv pip install --overrides /tmp/overrides.frontend.txt /tmp/benchmarks

FROM ${FRONTEND_IMAGE} AS pre_frontend

ARG PYTHON_VERSION
# Cache apt downloads; sharing=locked avoids apt/dpkg races with concurrent builds.
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    apt-get update -y \
    && apt-get install -y --no-install-recommends \
        # required for EPP
        ca-certificates \
        libstdc++6 \
        # required for verification of GPG keys
        gnupg2 \
        # compliance audit bootstraps syft over HTTPS
        curl \
        # Python runtime - required for virtual environment to work
        python${PYTHON_VERSION}-dev \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Bring base-image OS packages up to the current patch releases published in
# the distro archives. --only-upgrade skips anything not already installed, so
# no new packages are added; versions are left unpinned so a cache-busted
# rebuild picks up the newest patch level (BuildKit reuses this layer otherwise).
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    apt-get update -y \
    && apt-get install -y --no-install-recommends --only-upgrade \
        dirmngr \
        gnupg \
        gnupg-utils \
        gnupg2 \
        gpg \
        gpg-agent \
        gpgconf \
        gpgsm \
        gpgv \
        keyboxd \
        libssl3t64 \
        openssl \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Create dynamo user with group 0 for OpenShift compatibility
RUN userdel -r ubuntu > /dev/null 2>&1 || true \
    && useradd -m -s /bin/bash -g 0 dynamo \
    && [ `id -u dynamo` -eq 1000 ] \
    && mkdir -p /home/dynamo/.cache /opt/dynamo /workspace \
    && chown -R dynamo: /opt/dynamo /home/dynamo/.cache /workspace \
    && chmod -R g+w /opt/dynamo /home/dynamo/.cache /workspace

# Set HOME so ModelExpress can find the cache directory
ENV HOME=/home/dynamo
# Switch to dynamo user
USER dynamo
ENV DYNAMO_HOME=/opt/dynamo

WORKDIR /
COPY --chown=dynamo: --from=epp /epp /epp

COPY --chown=dynamo: container/launch_message/frontend.txt /opt/dynamo/.launch_screen
# Copy tests, benchmarks, deploy and components with correct ownership
COPY --chown=dynamo: tests /workspace/tests
COPY --chown=dynamo: examples /workspace/examples
COPY --chown=dynamo: benchmarks /workspace/benchmarks
COPY --chown=dynamo: deploy /workspace/deploy
COPY --chown=dynamo: dev /workspace/dev
COPY --chown=dynamo: components/ /workspace/components/
COPY --chown=dynamo: recipes/ /workspace/recipes/
# Copy LICENSE; ATTRIBUTIONS files removed in favor of /legal/ generated at build time.
COPY --chown=dynamo: LICENSE /workspace/

ENV VIRTUAL_ENV=/opt/dynamo/venv
ENV PATH="/opt/dynamo/venv/bin:$PATH"

# Copy uv from base stage and wheels from wheel_builder (no runtime stage dependency)
COPY --chown=dynamo: --from=dynamo_base /opt/uv/bin/uv /opt/uv/bin/uvx /opt/uv/bin/
ENV PATH=/opt/uv/bin:${PATH}
COPY --chown=dynamo: --from=wheel_builder /opt/dynamo/dist/*.whl /opt/dynamo/wheelhouse/
# crick wheel pre-built in the crick_builder stage; see comment near the top.
COPY --chown=dynamo: --from=crick_builder /wheels/ /opt/dynamo/wheelhouse/extra/

# The dependency builder carries the source-build toolchains; this stage receives
# only the completed environment.
COPY --chown=dynamo:0 --from=frontend_python_deps /opt/dynamo/venv /opt/dynamo/venv

ARG ENABLE_GPU_MEMORY_SERVICE
ARG NIXL_REF
# In an ideal world, we'd use a mirror of PyPI for much more reliable downloads.
# UV_FIND_LINKS points at the crick wheel pre-built in the crick_builder stage;
# uv prefers it over the sdist on arm64 where no manylinux aarch64 wheel exists.
RUN --mount=type=bind,source=./container/deps/overrides.frontend.txt,target=/tmp/overrides.frontend.txt \
    --mount=type=cache,id=uv-dynamo-0.12.0,target=/home/dynamo/.cache/uv,uid=1000,gid=0,mode=0775,sharing=shared \
    echo "${NIXL_REF}" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$' || { echo "NIXL_REF must be a vX.Y.Z release tag; got '${NIXL_REF}'" >&2; exit 1; } && \
    export UV_CACHE_DIR=/home/dynamo/.cache/uv UV_FIND_LINKS=/opt/dynamo/wheelhouse/extra && \
    uv pip install \
    --overrides /tmp/overrides.frontend.txt \
    /opt/dynamo/wheelhouse/ai_dynamo_runtime*.whl \
    /opt/dynamo/wheelhouse/ai_dynamo*any.whl && \
    # The meta package requires both backends unconditionally (its cu12/cu13
    # extras are vestigial), so install the backend first and the meta module
    # --no-deps to keep a single CUDA build and one libnixl_capi.so.
    uv pip install "nixl-cu13==${NIXL_REF#v}" && \
    uv pip install --no-deps "nixl==${NIXL_REF#v}" && \
    uv pip show nixl nixl-cu13 | grep -E '^(Name|Version)' | tee /opt/dynamo/nixl-versions.txt && \
    if [ "$ENABLE_GPU_MEMORY_SERVICE" = "true" ]; then \
        GMS_WHEEL=$(ls /opt/dynamo/wheelhouse/gpu_memory_service*.whl 2>/dev/null | head -1); \
        if [ -z "$GMS_WHEEL" ]; then \
            echo "ERROR: ENABLE_GPU_MEMORY_SERVICE is true but no gpu_memory_service wheel found in wheelhouse" >&2; \
            exit 1; \
        fi; \
        uv pip install "$GMS_WHEEL"; \
    fi

# Setup environment for all users
USER root
# nixl-sys resolves the C API with a bare dlopen("libnixl_capi.so"), and
# dynamo/_core.abi3.so carries no RPATH, so without this the Rust bindings
# silently fall back to stub mode while the Python ones work. The wheel keeps
# its libraries in a private directory; put that on the loader path. NIXL finds
# the backend plugins beside them on its own (nixl_plugin_manager.cpp's
# getPluginDir dladdr fallback), so NIXL_PLUGIN_DIR is deliberately left unset.
RUN NIXL_LIB_DIR="$(/opt/dynamo/venv/bin/python -c 'import sysconfig; print(sysconfig.get_paths()["purelib"])')/.nixl_cu13.mesonpy.libs" && \
    [ -f "${NIXL_LIB_DIR}/libnixl_capi.so" ] || { echo "missing ${NIXL_LIB_DIR}/libnixl_capi.so; NIXL wheel layout changed" >&2; exit 1; } && \
    [ -d "${NIXL_LIB_DIR}/plugins" ] || { echo "missing ${NIXL_LIB_DIR}/plugins; NIXL wheel layout changed" >&2; exit 1; } && \
    echo "${NIXL_LIB_DIR}" > /etc/ld.so.conf.d/nixl.conf && \
    ldconfig && \
    chmod 755 /opt/dynamo/.launch_screen && \
    echo 'source /opt/dynamo/venv/bin/activate' >> /etc/bash.bashrc && \
    echo 'cat /opt/dynamo/.launch_screen' >> /etc/bash.bashrc

USER dynamo

ENTRYPOINT ["/epp"]
CMD ["/bin/bash"]

# === BEGIN templates/compliance.Dockerfile ===
#
# Inline-compliance Dockerfile stages, shared by the vllm / sglang / trtllm
# runtime templates.
#
# This template emits four stages in fixed order:
#
#   1. licenses          -- runs compliance.generators against the
#                           previously-defined build stage, validates output
#                           against policy, and stages the unified /legal tree
#                           (flat NOTICES-<Eco>.txt + osrb-deps.csv + osrb.cdx.json).
#   2. compliance_artifact -- FROM scratch; exposes the unified /legal tree for
#                           CI extraction as a single `-compliance` artifact.
#                           (Named *_artifact to avoid colliding with the
#                           `compliance` build-context the deploy Dockerfiles use.)
#   3. sources_collect   -- gated on ENABLE_SOURCE_ARCHIVAL; runs
#                           compliance.collect_sources to produce /sources.zip.
#   4. sources_archive   -- FROM scratch; exposes /sources.zip.
#
# The caller (each per-framework runtime template) is expected to:
#   - have defined `pre_runtime` already
#   - end with its own final stage (typically `runtime`) that does
#     `COPY --from=licenses /legal /legal` to inherit NOTICES.
#
# Jinja variables consumed:
#
#   compliance_base_stage     -- "pre_runtime"; set by
#                                container/render.py:_render_context().
#   compliance_baseline_sbom  -- filename under base_sboms/ (or empty string
#                                if no baseline captured); set by
#                                _render_context() from `framework`/
#                                `device_key`.
#   compliance_ecosystems     -- comma-separated --ecosystem list for the
#                                licenses stage. planner drops dpkg (distroless,
#                                ships no builder Debian packages); other targets
#                                get python,rust,dpkg,native. Set by
#                                _render_context().
#   compliance_source_ecosystem_flags -- repeated --ecosystem flags for the
#                                sources_collect stage; per-target likewise.
#   framework, target, make_efa -- already in render context; control
#                                  ecosystem flags + EFA native attribution.

#######################################
########## Compliance: licenses #######
#######################################
#
# Runs every per-ecosystem generator under container/compliance/generators/
# against the parent build stage's filesystem, applies the license policy
# gate, and exposes /legal/ + /sboms/ for the next two stages to fan out.
#
# Per-framework variations:
#   - sglang uses `--site-packages "$(... sysconfig ...)"` because the
#     upstream image installs into system Python via
#     `pip install --break-system-packages`, not a venv.
#   - native always runs (image filter "{framework}-{target}[-efa]"), attributing
#     the from-source binaries the python/rust/dpkg scanners miss (ffmpeg, libvpx,
#     UCX, NIXL, gdrcopy, libfabric, etcd, nats-server) per native_packages.yaml's
#     per-image `images:` lists; make_efa adds the EFA-only entries.

FROM pre_frontend AS licenses

USER root
RUN mkdir -p /legal /sboms
COPY --chown=root:0 container/compliance /opt/compliance
ENV PYTHONPATH=/opt

# Real crate LICENSE files harvested from the cargo registry in wheel_builder
# (empty when none were harvested -- the rust generator then falls back to
# canonical SPDX text). Keyed "<name>-<version>". wheel_builder_base always
# creates the dir, so this COPY never fails even for wheel-less targets.
COPY --from=wheel_builder /opt/dynamo/rust-licenses /tmp/rust-licenses

# EPP's Go compliance SBOM + harvested module LICENSE files, read from the build
# CONTEXT (.epp-sbom/) rather than COPY --from the EPP image. The CI EPP-build
# step exports them there via `make sbom-export` while the build cache is warm
# (see deploy/inference-gateway/epp/Dockerfile sbom-export stage). This avoids
# re-pulling the pushed EPP image — whose runtime layer could be served from a
# stale cache and miss these files after the BuildKit builder is refreshed. The
# SBOM is exported once on amd64; EPP's Go module set doesn't vary by GOARCH
# (linux only), so the amd64 export is authoritative for all frontend arches.
COPY --from=epp_go_builder /sbom-go.cdx.json /tmp/sbom-go-epp.cdx.json
# Real Go module LICENSE files so the go generator inlines upstream license text
# instead of canonical SPDX fallback.
COPY --from=epp_go_builder /sbom-go-licenses /tmp/go-licenses

# BASELINE_SBOM_FILE: the per-arch baseline SBOM *stem* (e.g.
# "cuda@2ab6381d") under /opt/compliance/base_sboms/. We append
# "-${TARGETARCH}.cdx.json" so each platform of a multi-arch build subtracts
# its OWN-arch floor — the amd64 baseline would otherwise under-attribute a
# package present in the amd64 base but not the arm64 base that we install on
# arm64. Rendered from context.yaml's baseline_sbom by render.py; empty when no
# baseline is captured (NOTICES then cover the full image — correct but
# unfiltered).
ARG BASELINE_SBOM_FILE="ubuntu@7291df36"
ARG TARGETARCH
# Resolve where this image's Python packages live at runtime rather than per
# framework: venv-based images export VIRTUAL_ENV (trtllm, vllm xpu/cpu, dev),
# while images that install into system Python leave it unset (vllm cuda via
# `pip --system`, sglang via `pip --break-system-packages`). Pick the matching
# generator flag so it always finds the deps — passing an empty
# `--venv ${VIRTUAL_ENV}` is what broke system-Python images.
RUN if [ -n "${VIRTUAL_ENV:-}" ]; then PKG_ARG="--venv ${VIRTUAL_ENV}"; else PKG_ARG="--site-packages $(python3 -c 'import sysconfig; print(sysconfig.get_paths()["purelib"])')"; fi && \
    python3 -m compliance.generators \
    --ecosystem python,rust,dpkg,go,native \
    ${PKG_ARG} \
    --go-sbom /tmp/sbom-go-epp.cdx.json \
    --go-licenses-dir /tmp/go-licenses \
    --rust-licenses-dir /tmp/rust-licenses \
    --output-dir /legal \
    --policy /opt/compliance/policy/licenses.toml \
    --native-yaml /opt/compliance/native_packages.yaml \
    --native-image dynamo-frontend \
    ${BASELINE_SBOM_FILE:+--subtract-sbom /opt/compliance/base_sboms/${BASELINE_SBOM_FILE}-${TARGETARCH}.cdx.json} \
    -v
# Policy gate runs on the single unified CSV (its `ecosystem` column scopes each
# row), replacing the per-ecosystem loop. Non-zero exit fails the build.
RUN python3 -m compliance.policy.validate \
        --policy /opt/compliance/policy/licenses.toml \
        --input /legal/osrb-deps.csv

# Media-codec allowlist gate: scans THIS stage's filesystem (==
# the shipped image tree, since licenses is FROM pre_runtime) and fails the build
# if a media-codec library/binary (a third-party libav*, libx264/265, or a stray
# or imageio-bundled ffmpeg) ships outside our in-tree allowlist or a reasoned
# exception. Feeds the generated delta SBOM in too, for an ffmpeg-version floor.
# Files, not just the SBOM, because statically-bundled codec .so's don't appear
# as components.
#
# CUDA only for now. The XPU image is not currently published on NGC, so it does
# not yet require the codecs to be removed. It would also fail this gate today:
# the purge the gate depends on sits in the `device == "cuda"` block of
# vllm_runtime.Dockerfile, so the XPU image still carries its base image's media
# stack. Enable both together if that changes.

RUN python3 -m compliance.scan_codecs \
        --root / \
        --policy /opt/compliance/policy/codec_policy.yaml \
        --sbom /legal/osrb.cdx.json \
        --image dynamo-frontend \
        --fail-on-findings -v

#######################################
####### Compliance: artifact ##########
#######################################
#
# Single FROM-scratch stage exposing the unified compliance tree for CI
# extraction (one `-compliance` artifact): flat NOTICES-<Eco>.txt + the unified
# osrb-deps.csv (with Notes) + osrb.cdx.json (delta CycloneDX). Export is bounded
# by these files' size (a few MB) regardless of runtime image size.

FROM scratch AS compliance_artifact
COPY --from=licenses /legal/ /

#######################################
########## Compliance: sources ########
#######################################
#
# Collects third-party source archives on top of the runtime baseline.
# Gated on ENABLE_SOURCE_ARCHIVAL -- default off so PR builds stay fast;
# CI flips it on for nightly + release/*.*.* branch pushes (see
# .github/workflows/post-merge-ci.yml and nightly-ci.yml).

FROM pre_frontend AS sources_collect

USER root
RUN mkdir -p /sources /opt/compliance /opt/native-sources /opt/dynamo-vendor-full
COPY --chown=root:0 container/compliance /opt/compliance
ENV PYTHONPATH=/opt
COPY --from=wheel_builder /tmp/native-sources/ /opt/native-sources/
COPY --from=wheel_builder /tmp/dynamo-vendor-full/ /opt/dynamo-vendor-full/

ARG ENABLE_SOURCE_ARCHIVAL=false
ARG BASELINE_SBOM_FILE="ubuntu@7291df36"
ARG TARGETARCH
RUN if [ "$ENABLE_SOURCE_ARCHIVAL" = "true" ]; then \
if [ -n "${VIRTUAL_ENV:-}" ]; then RUST_PKG_ARG="--rust-venv ${VIRTUAL_ENV}"; else RUST_PKG_ARG="--rust-site-packages $(python3 -c 'import sysconfig; print(sysconfig.get_paths()["purelib"])')"; fi && \
        python3 -m compliance.collect_sources \
            --ecosystem dpkg --ecosystem rust --ecosystem native --ecosystem go \
            --output-zip /sources.zip \
            --sources-root /sources \
            --native-source-dir /opt/native-sources \
            ${RUST_PKG_ARG} \
            --rust-vendor-full /opt/dynamo-vendor-full \
            ${BASELINE_SBOM_FILE:+--baseline-sbom /opt/compliance/base_sboms/${BASELINE_SBOM_FILE}-${TARGETARCH}.cdx.json} \
            -v ; \
    else \
        python3 -c "import zipfile; zipfile.ZipFile('/sources.zip','w').close()" ; \
    fi

FROM scratch AS sources_archive
COPY --from=sources_collect /sources.zip /sources.zip

# === END templates/compliance.Dockerfile ===

FROM pre_frontend AS frontend
COPY --from=licenses /legal /legal
ENTRYPOINT ["/epp"]
CMD ["/bin/bash"]

# --- SGLang Stages

# --- No SGLANG stages included

# --- VLLM Stages

# --- No VLLM stages included

# --- TRTLLM Stages

# --- No TRTLLM stages included

# --- Development Stages

# --- No development stages included
