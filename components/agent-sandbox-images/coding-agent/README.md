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
