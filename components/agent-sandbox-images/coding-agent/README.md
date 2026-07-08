# Coding agent sandbox image

This image is intended for `SandboxTemplate` pods that execute `FactoryTask`
work. It includes common coding tools (`git`, Go, Node.js, Python, `ripgrep`,
`jq`, `make`, `curl`) and installs Gemini CLI by default.

Build locally:

```bash
docker build -t ai-factory/coding-agent-sandbox:dev components/agent-sandbox-images/coding-agent
```

Load into kind:

```bash
kind load docker-image ai-factory/coding-agent-sandbox:dev --name ai-factory-ci
```

Use it from a `SandboxTemplate`:

```yaml
containers:
- name: dev
  image: ai-factory/coding-agent-sandbox:dev
  imagePullPolicy: IfNotPresent
  command: ["/bin/bash", "-lc", "sleep 3600"]
  workingDir: /workspace
```

The wrapper command `ai-factory-agent` prefers `gemini --yolo` when Gemini CLI
is present and falls back to `codex` if installed. Build with
`--build-arg INSTALL_CODEX_CLI=true` to install the Codex CLI package too.

Use `ai-factory-agent openai-compatible` for providers that expose an
OpenAI-compatible `/chat/completions` API. The command reads the FactoryTask
prompt from stdin, asks the model to generate a focused shell script, and runs
that script in the current repository. Configure it with `OPENAI_API_KEY`,
`OPENAI_BASE_URL`, and `OPENAI_MODEL`. For example, Kimi can be configured with
`OPENAI_BASE_URL=https://api.moonshot.cn/v1` and the provider's model name.
