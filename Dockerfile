ARG BASE_IMAGE=ubuntu:26.04
ARG INTEL_BASE_IMAGE=ubuntu:22.04
ARG UBUNTU_CODENAME=noble
# Optional alternate Ubuntu apt mirror(s). Empty = use upstream.
# See .docker/apt-mirror.sh for accepted values.
ARG APT_MIRROR=""
ARG APT_PORTS_MIRROR=""

FROM ${BASE_IMAGE} AS requirements

ARG APT_MIRROR
ARG APT_PORTS_MIRROR
ENV DEBIAN_FRONTEND=noninteractive

RUN --mount=type=bind,source=.docker/apt-mirror.sh,target=/usr/local/sbin/apt-mirror \
    APT_MIRROR="${APT_MIRROR}" APT_PORTS_MIRROR="${APT_PORTS_MIRROR}" sh /usr/local/sbin/apt-mirror && \
    apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates curl wget espeak-ng libgomp1 gosu \
        ffmpeg libopenblas0 libopenblas-dev libopus0 sox && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# The requirements-drivers target is for BUILD_TYPE specific items.  If you need to install something specific to CUDA, or specific to ROCM, it goes here.
FROM requirements AS requirements-drivers

ARG BUILD_TYPE
ARG CUDA_MAJOR_VERSION=13
ARG CUDA_MINOR_VERSION=3
ARG UBUNTU_CODENAME
ARG SKIP_DRIVERS=false
ARG TARGETARCH
ARG TARGETVARIANT
ENV BUILD_TYPE=${BUILD_TYPE}
ARG UBUNTU_VERSION=2604

RUN mkdir -p /run/localai
RUN echo "default" > /run/localai/capability

# Vulkan requirements
RUN set -e; if [ "${BUILD_TYPE}" = "vulkan" ] && [ "${SKIP_DRIVERS}" = "false" ]; then \
        apt-get update; \
        apt-get install -y --no-install-recommends \
            software-properties-common pciutils wget gpg-agent; \
        apt-get install -y --no-install-recommends libglm-dev cmake libxcb-dri3-0 libxcb-present0 libpciaccess0 \
            libpng-dev libxcb-keysyms1-dev libxcb-dri3-dev libx11-dev libmirclient-dev \
            libwayland-dev libxrandr-dev libxcb-randr0-dev libxcb-ewmh-dev \
            git python3 bison pkg-config; \
        wget -qO - https://packages.lunarg.com/lunarg-signing-key-pub.asc | gpg --dearmor -o /usr/share/keyrings/lunarg-vulkan.gpg; \
        echo "deb [signed-by=/usr/share/keyrings/lunarg-vulkan.gpg] https://packages.lunarg.com/vulkan ${UBUNTU_CODENAME} main" > /etc/apt/sources.list.d/lunarg-vulkan.list; \
        apt-get update; \
        apt-get install -y --no-install-recommends vulkan-sdk; \
        if [ "amd64" = "$TARGETARCH" ]; then \
            wget "https://sdk.lunarg.com/sdk/download/1.4.335.0/linux/vulkansdk-linux-x86_64-1.4.335.0.tar.xz" && \
            tar -xf vulkansdk-linux-x86_64-1.4.335.0.tar.xz && \
            rm vulkansdk-linux-x86_64-1.4.335.0.tar.xz && \
            mkdir -p /opt/vulkan-sdk && \
            mv 1.4.335.0 /opt/vulkan-sdk/ && \
            cd /opt/vulkan-sdk/1.4.335.0 && \
            ./vulkansdk --no-deps --maxjobs \
                vulkan-loader \
                vulkan-validationlayers \
                vulkan-extensionlayer \
                vulkan-tools \
                shaderc && \
            cp -rfv /opt/vulkan-sdk/1.4.335.0/x86_64/bin/* /usr/bin/ && \
            cp -rfv /opt/vulkan-sdk/1.4.335.0/x86_64/lib/* /usr/lib/x86_64-linux-gnu/ && \
            cp -rfv /opt/vulkan-sdk/1.4.335.0/x86_64/include/* /usr/include/ && \
            cp -rfv /opt/vulkan-sdk/1.4.335.0/x86_64/share/* /usr/share/ && \
            rm -rf /opt/vulkan-sdk; \
        fi; \
        if [ "arm64" = "$TARGETARCH" ]; then \
            mkdir vulkan && cd vulkan && \
            curl -L -o vulkan-sdk.tar.xz https://github.com/mudler/vulkan-sdk-arm/releases/download/1.4.335.0/vulkansdk-ubuntu-24.04-arm-1.4.335.0.tar.xz && \
            tar -xvf vulkan-sdk.tar.xz && \
            rm vulkan-sdk.tar.xz && \
            cd 1.4.335.0 && \
            cp -rfv aarch64/bin/* /usr/bin/ && \
            cp -rfv aarch64/lib/* /usr/lib/aarch64-linux-gnu/ && \
            cp -rfv aarch64/include/* /usr/include/ && \
            cp -rfv aarch64/share/* /usr/share/ && \
            cd ../.. && \
            rm -rf vulkan; \
        fi; \
        ldconfig && \
        apt-get clean && \
        rm -rf /var/lib/apt/lists/* && \
        echo "vulkan" > /run/localai/capability; \
    fi

# CuBLAS requirements
RUN set -e; if ( [ "${BUILD_TYPE}" = "cublas" ] || [ "${BUILD_TYPE}" = "l4t" ] ) && [ "${SKIP_DRIVERS}" = "false" ]; then \
        apt-get update; \
        apt-get install -y --no-install-recommends software-properties-common pciutils; \
        if [ "amd64" = "$TARGETARCH" ]; then \
            curl -fO https://developer.download.nvidia.com/compute/cuda/repos/ubuntu${UBUNTU_VERSION}/x86_64/cuda-keyring_1.1-1_all.deb; \
        fi; \
        if [ "arm64" = "$TARGETARCH" ]; then \
            if [ "${CUDA_MAJOR_VERSION}" = "13" ]; then \
                curl -fO https://developer.download.nvidia.com/compute/cuda/repos/ubuntu${UBUNTU_VERSION}/sbsa/cuda-keyring_1.1-1_all.deb; \
            else \
                curl -fO https://developer.download.nvidia.com/compute/cuda/repos/ubuntu${UBUNTU_VERSION}/arm64/cuda-keyring_1.1-1_all.deb; \
            fi; \
        fi; \
        dpkg -i cuda-keyring_1.1-1_all.deb; \
        rm -f cuda-keyring_1.1-1_all.deb; \
        apt-get update; \
        apt-get install -y --no-install-recommends \
            cuda-nvcc-${CUDA_MAJOR_VERSION}-${CUDA_MINOR_VERSION} \
            cuda-nvrtc-dev-${CUDA_MAJOR_VERSION}-${CUDA_MINOR_VERSION} \
            libcufft-dev-${CUDA_MAJOR_VERSION}-${CUDA_MINOR_VERSION} \
            libcurand-dev-${CUDA_MAJOR_VERSION}-${CUDA_MINOR_VERSION} \
            libcublas-dev-${CUDA_MAJOR_VERSION}-${CUDA_MINOR_VERSION} \
            libcusparse-dev-${CUDA_MAJOR_VERSION}-${CUDA_MINOR_VERSION} \
            libcusolver-dev-${CUDA_MAJOR_VERSION}-${CUDA_MINOR_VERSION}; \
        if [ "${CUDA_MAJOR_VERSION}" = "13" ] && [ "arm64" = "$TARGETARCH" ]; then \
            apt-get install -y --no-install-recommends \
            libcufile-${CUDA_MAJOR_VERSION}-${CUDA_MINOR_VERSION} libcudnn9-cuda-${CUDA_MAJOR_VERSION} cuda-cupti-${CUDA_MAJOR_VERSION}-${CUDA_MINOR_VERSION} libnvjitlink-${CUDA_MAJOR_VERSION}-${CUDA_MINOR_VERSION}; \
        fi; \
        apt-get clean; \
        rm -rf /var/lib/apt/lists/*; \
        echo "nvidia-cuda-${CUDA_MAJOR_VERSION}" > /run/localai/capability; \
    fi

RUN if [ "${BUILD_TYPE}" = "cublas" ] && [ "${TARGETARCH}" = "arm64" ]; then \
        echo "nvidia-l4t-cuda-${CUDA_MAJOR_VERSION}" > /run/localai/capability; \
    fi

# https://github.com/NVIDIA/Isaac-GR00T/issues/343
RUN if [ "${BUILD_TYPE}" = "cublas" ] && [ "${TARGETARCH}" = "arm64" ]; then \
        wget https://developer.download.nvidia.com/compute/cudss/0.6.0/local_installers/cudss-local-tegra-repo-ubuntu${UBUNTU_VERSION}-0.6.0_0.6.0-1_arm64.deb && \
        dpkg -i cudss-local-tegra-repo-ubuntu${UBUNTU_VERSION}-0.6.0_0.6.0-1_arm64.deb && \
        cp /var/cudss-local-tegra-repo-ubuntu${UBUNTU_VERSION}-0.6.0/cudss-*-keyring.gpg /usr/share/keyrings/ && \
        apt-get update && apt-get -y install cudss cudss-cuda-${CUDA_MAJOR_VERSION} && \
        wget https://developer.download.nvidia.com/compute/nvpl/25.5/local_installers/nvpl-local-repo-ubuntu${UBUNTU_VERSION}-25.5_1.0-1_arm64.deb && \
        dpkg -i nvpl-local-repo-ubuntu${UBUNTU_VERSION}-25.5_1.0-1_arm64.deb && \
        cp /var/nvpl-local-repo-ubuntu${UBUNTU_VERSION}-25.5/nvpl-*-keyring.gpg /usr/share/keyrings/ && \
        apt-get update && apt-get install -y nvpl; \
    fi

# If we are building with clblas support, we need the libraries for the builds
RUN if [ "${BUILD_TYPE}" = "clblas" ] && [ "${SKIP_DRIVERS}" = "false" ]; then \
        apt-get update && \
        apt-get install -y --no-install-recommends \
            libclblast-dev && \
        apt-get clean && \
        rm -rf /var/lib/apt/lists/* \
    ; fi

RUN if [ "${BUILD_TYPE}" = "hipblas" ] && [ "${SKIP_DRIVERS}" = "false" ]; then \
        apt-get update && \
        apt-get install -y --no-install-recommends \
            hipblas-dev \
            hipblaslt-dev \
            rocblas-dev && \
        apt-get clean && \
        rm -rf /var/lib/apt/lists/* && \
        echo "amd" > /run/localai/capability && \
        # I have no idea why, but the ROCM lib packages don't trigger ldconfig after they install, which results in local-ai and others not being able
        # to locate the libraries. We run ldconfig ourselves to work around this packaging deficiency
        ldconfig \
    ; fi

RUN if [ "${BUILD_TYPE}" = "hipblas" ]; then \
    ln -s /opt/rocm-**/lib/llvm/lib/libomp.so /usr/lib/libomp.so \
    ; fi

# ROCm's bundled libdrm_amdgpu is built with a hardcoded fallback lookup path
# for the ASIC ID table (/opt/amdgpu/share/libdrm/amdgpu.ids), which only exists
# if AMD's full amdgpu graphics/DKMS stack is installed. This compute-only image
# doesn't have it, so hipblas/rocBLAS log "No such file or directory" on every
# model load and can fail to identify the GPU. Point it at the equivalent file
# Ubuntu's libdrm-common package already ships.
RUN if [ "${BUILD_TYPE}" = "hipblas" ] && [ -f /usr/share/libdrm/amdgpu.ids ] && [ ! -e /opt/amdgpu/share/libdrm/amdgpu.ids ]; then \
    mkdir -p /opt/amdgpu/share/libdrm && \
    ln -s /usr/share/libdrm/amdgpu.ids /opt/amdgpu/share/libdrm/amdgpu.ids \
    ; fi

# Intel requirements
# Temporary workaround for Intel's repository to work correctly
RUN set -e; if [ "${BUILD_TYPE}" = "intel" ] && [ "${SKIP_DRIVERS}" = "false" ]; then \
        apt-get update; \
        apt-get install -y --no-install-recommends gnupg; \
        wget -qO - https://repositories.intel.com/gpu/intel-graphics.key | \
        gpg --yes --dearmor --output /usr/share/keyrings/intel-graphics.gpg; \
        echo "deb [arch=amd64 signed-by=/usr/share/keyrings/intel-graphics.gpg] https://repositories.intel.com/gpu/ubuntu ${UBUNTU_CODENAME}/lts/2350 unified" > /etc/apt/sources.list.d/intel-graphics.list; \
        apt-get update; \
        apt-get install -y --no-install-recommends intel-opencl-icd intel-level-zero-gpu level-zero; \
        apt-get clean; \
        rm -rf /var/lib/apt/lists/*; \
    fi

RUN expr "${BUILD_TYPE}" = intel && echo "intel" > /run/localai/capability || echo "not intel"

# Cuda
ENV PATH=/usr/local/cuda/bin:${PATH}

# HipBLAS requirements
ENV PATH=/opt/rocm/bin:${PATH}

###################################
###################################

# The requirements-core target is common to all images.  It should not be placed in requirements-core unless every single build will use it.
FROM requirements-drivers AS build-requirements

ARG GO_VERSION=1.26.5
ARG CMAKE_VERSION=3.31.10
ARG CMAKE_FROM_SOURCE=false
ARG TARGETARCH
ARG TARGETVARIANT

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        build-essential \
        ccache \
        ca-certificates espeak-ng \
        curl libssl-dev \
        git \
        git-lfs \
        libopus-dev pkg-config \
        unzip upx-ucl python3 python-is-python3 && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install CMake (the version in 22.04 is too old)
RUN if [ "${CMAKE_FROM_SOURCE}" = "true" ]; then \
        curl -L -s https://github.com/Kitware/CMake/releases/download/v${CMAKE_VERSION}/cmake-${CMAKE_VERSION}.tar.gz -o cmake.tar.gz && tar xvf cmake.tar.gz && cd cmake-${CMAKE_VERSION} && ./configure && make && make install; \
    else \
        apt-get update && \
        apt-get install -y \
            cmake && \
        apt-get clean && \
        rm -rf /var/lib/apt/lists/*; \
    fi

# Install Go
RUN curl -L -s https://go.dev/dl/go${GO_VERSION}.linux-${TARGETARCH}.tar.gz | tar -C /usr/local -xz
ENV PATH=/root/go/bin:/usr/local/go/bin:$PATH

# Install grpc compilers
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2 && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@1958fcbe2ca8bd93af633f11e97d44e567e945af

COPY --chmod=644 custom-ca-certs/* /usr/local/share/ca-certificates/
RUN update-ca-certificates

RUN test -n "$TARGETARCH" \
    || (echo 'warn: missing $TARGETARCH, either set this `ARG` manually, or run using `docker buildkit`')

# Use the variables in subsequent instructions
RUN echo "Target Architecture: $TARGETARCH"
RUN echo "Target Variant: $TARGETVARIANT"




WORKDIR /build


###################################
###################################



###################################
###################################

# The builder-base target has the arguments, variables, and copies shared between full builder images and the uncompiled devcontainer

FROM build-requirements AS builder-base

ARG GO_TAGS="auth"
ARG GRPC_BACKENDS
ARG MAKEFLAGS
ARG LD_FLAGS="-s -w"
ARG TARGETARCH
ARG TARGETVARIANT
ENV GRPC_BACKENDS=${GRPC_BACKENDS}
ENV GO_TAGS=${GO_TAGS}
ENV MAKEFLAGS=${MAKEFLAGS}
ENV NVIDIA_DRIVER_CAPABILITIES=compute,utility
ENV NVIDIA_REQUIRE_CUDA="cuda>=${CUDA_MAJOR_VERSION}.0"
ENV NVIDIA_VISIBLE_DEVICES=all
ENV LD_FLAGS=${LD_FLAGS}

RUN echo "GO_TAGS: $GO_TAGS" && echo "TARGETARCH: $TARGETARCH"

WORKDIR /build


# We need protoc installed, and the version in 22.04 is too old.
RUN if [ "amd64" = "$TARGETARCH" ]; then \
        curl -L -s https://github.com/protocolbuffers/protobuf/releases/download/v27.1/protoc-27.1-linux-x86_64.zip -o protoc.zip && \
        unzip -j -d /usr/local/bin protoc.zip bin/protoc && \
        rm protoc.zip; \
    fi; \
    if [ "arm64" = "$TARGETARCH" ]; then \
        curl -L -s https://github.com/protocolbuffers/protobuf/releases/download/v27.1/protoc-27.1-linux-aarch_64.zip -o protoc.zip && \
        unzip -j -d /usr/local/bin protoc.zip bin/protoc && \
        rm protoc.zip; \
    fi

###################################
###################################

# Build React UI
FROM node:26-slim AS react-ui-builder
WORKDIR /app
COPY core/http/react-ui/package*.json ./
RUN npm install
COPY core/http/react-ui/ ./
RUN npm run build

###################################
###################################

# Compile backends first in a separate stage
FROM builder-base AS builder-backends
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /build

COPY ./Makefile .
COPY ./backend ./backend
COPY ./go.mod .
COPY ./go.sum .
COPY ./.git ./.git

# Some of the Go backends use libs from the main src, we could further optimize the caching by building the CPP backends before here
COPY ./pkg/grpc ./pkg/grpc
COPY ./pkg/utils ./pkg/utils

RUN ls -l ./
RUN make protogen-go

# The builder target compiles LocalAI. This target is not the target that will be uploaded to the registry.
# Adjustments to the build process should likely be made here.
FROM builder-backends AS builder

WORKDIR /build

COPY . .

# Copy pre-built React UI
COPY --from=react-ui-builder /app/dist ./core/http/react-ui/dist

## Build the binary
## If we're on arm64 AND using cublas/hipblas, skip some of the llama-compat backends to save space
## Otherwise just run the normal build
RUN make build

###################################
###################################

# The devcontainer target is not used on CI. It is a target for developers to use locally -
# rather than copying files it mounts them locally and leaves building to the developer

FROM builder-base AS devcontainer

COPY .devcontainer-scripts /.devcontainer-scripts

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ssh less
# For the devcontainer, leave apt functional in case additional devtools are needed at runtime.

RUN go install github.com/go-delve/delve/cmd/dlv@latest

RUN go install github.com/mikefarah/yq/v4@latest

###################################
###################################

# This is the final target. The result of this target will be the image uploaded to the registry.
# If you cannot find a more suitable place for an addition, this layer is a suitable place for it.
FROM requirements-drivers

ENV HEALTHCHECK_ENDPOINT=http://localhost:8080/readyz

ARG CUDA_MAJOR_VERSION=13
ENV NVIDIA_DRIVER_CAPABILITIES=compute,utility
ENV NVIDIA_REQUIRE_CUDA="cuda>=${CUDA_MAJOR_VERSION}.0"
ENV NVIDIA_VISIBLE_DEVICES=all

# Create a localai user and group, robustly handling if UID/GID 1000 already exists (e.g. 'ubuntu' in Ubuntu 24.04+)
RUN if getent group 1000 >/dev/null; then \
        EXISTING_GROUP=$(getent group 1000 | cut -d: -f1); \
        groupmod -n localai "$EXISTING_GROUP"; \
    else \
        groupadd -g 1000 localai; \
    fi && \
    if getent passwd 1000 >/dev/null; then \
        EXISTING_USER=$(getent passwd 1000 | cut -d: -f1); \
        usermod -l localai -d /home/localai -m "$EXISTING_USER"; \
    else \
        useradd -u 1000 -g localai -m -s /bin/bash localai; \
    fi && \
    # Ensure video and render groups exist (they are used for GPU pass-through)
    getent group video >/dev/null || groupadd video && \
    getent group render >/dev/null || groupadd render && \
    usermod -aG video,render localai

WORKDIR /

# Make sure key directories exist and are owned by localai
RUN mkdir -p /models /backends /data /configuration /run/localai && \
    chown -R localai:localai /models /backends /data /configuration /run/localai

COPY --chown=localai:localai ./entrypoint.sh .

# Copy the binary
COPY --chown=localai:localai --from=builder /build/local-ai ./
# Copy the opus shim if it was built
RUN --mount=from=builder,src=/build/,dst=/mnt/build \
    if [ -f /mnt/build/libopusshim.so ]; then cp /mnt/build/libopusshim.so ./ && chown localai:localai ./libopusshim.so; fi

# Define the health check command
HEALTHCHECK --interval=1m --timeout=10m --retries=10 \
  CMD curl -f ${HEALTHCHECK_ENDPOINT} || exit 1

VOLUME /models /backends /configuration /data
EXPOSE 8080
ENTRYPOINT [ "/entrypoint.sh" ]
