# Coding agent sandbox image

This image is intended for SandboxTemplate pods that execute FactoryTask work.
It includes common coding tools such as git, Go, Node.js, Python, ripgrep, jq,
make, and curl, plus the provider-neutral ai-factory-agent wrapper.

Build locally:

    docker build -t ai-factory/coding-agent-sandbox:dev components/agent-sandbox-images/coding-agent

Load into kind:

    kind load docker-image ai-factory/coding-agent-sandbox:dev --name ai-factory-ci

Use it from a SandboxTemplate with a long-running shell command and working
directory /workspace.

The default runner is:

    ai-factory-agent openai-compatible

It reads the FactoryTask prompt from stdin, calls an OpenAI-compatible
chat/completions endpoint, asks the selected model for a focused shell script,
and executes that script in the checked-out repository.

Provider configuration:

- OPENAI_API_KEY: required by the OpenAI-compatible runner.
- OPENAI_BASE_URL: endpoint base URL, defaulting to the public OpenAI API.
- OPENAI_MODEL: provider model name.
- OPENAI_TEMPERATURE, OPENAI_MAX_TOKENS, OPENAI_MAX_TOOL_ROUNDS,
  OPENAI_MAX_FINAL_SCRIPT_ROUNDS, and OPENAI_MAX_REPAIR_ROUNDS: execution limits.

Other agent CLIs can be selected with AGENT_COMMAND or spec.agent.command.
The image does not require or assume a particular model provider. The optional
INSTALL_CODEX_CLI build argument installs the Codex CLI adapter, but the
OpenAI-compatible runner remains the default.
