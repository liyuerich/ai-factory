# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM node:22-bookworm

ARG DEBIAN_FRONTEND=noninteractive
ARG INSTALL_GEMINI_CLI=true
ARG INSTALL_CODEX_CLI=false

ENV HOME=/home/agent \
    WORKSPACE=/workspace \
    npm_config_update_notifier=false \
    npm_config_fund=false \
    npm_config_audit=false

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        bash \
        ca-certificates \
        coreutils \
        curl \
        dnsutils \
        git \
        golang-go \
        jq \
        less \
        make \
        nano \
        openssh-client \
        python3 \
        python3-pip \
        ripgrep \
        tar \
        unzip \
        vim-tiny \
        xz-utils \
    && rm -rf /var/lib/apt/lists/*

RUN if [ "${INSTALL_GEMINI_CLI}" = "true" ]; then \
        npm install -g @google/gemini-cli; \
    fi \
    && if [ "${INSTALL_CODEX_CLI}" = "true" ]; then \
        npm install -g @openai/codex; \
    fi \
    && npm cache clean --force

COPY ai-factory-agent /usr/local/bin/ai-factory-agent
COPY entrypoint.sh /usr/local/bin/ai-factory-sandbox-entrypoint

RUN chmod 755 /usr/local/bin/ai-factory-agent /usr/local/bin/ai-factory-sandbox-entrypoint \
    && useradd --create-home --home-dir "${HOME}" --shell /bin/bash agent \
    && mkdir -p "${WORKSPACE}" \
    && chown -R agent:agent "${HOME}" "${WORKSPACE}"

USER agent
WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/ai-factory-sandbox-entrypoint"]
CMD ["sleep", "3600"]
