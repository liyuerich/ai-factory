# AI Factory

This project is an experiment in whether a coding agent can self-assemble. In other words, can a coding agent build an unattended coding-agent that can "brainstorm" ideas, open issues, create PRs to fix those issues, merge them automatically, and iterate towards an end state.

This project's end state is "self-hosting": building the coding agent that can perform these tasks autonomously.

We will rely on a Kubernetes cluster and run agents in sandboxes provided by [agent-sandbox](https://github.com/kubernetes-sigs/agent-sandbox). GKE is the first supported managed environment, but the install flow is intended to work with any Kubernetes cluster reachable through the current `kubectl` context. We will initially use `gemini-cli` as our coding agent. We will assume this infrastructure exists initially, but will work towards building the infrastructure to run this experiment purely agentically.

## Repository Structure

- `.agents/`: Definitions and instructions for the various AI agents that operate within this repository (e.g., `builder`, `planner`, `reviewer`, `speccer`). Each agent has its own `agent.md` defining its persona, tools, and goals.
- `components/`: Software components intended for installation on Kubernetes. Each component has its own installation logic (e.g., `components/<name>/install`), which is invoked by the master `components/install` script. This includes infrastructure like `agent-sandbox` which provides the Kubernetes-native sandboxes where our AI agents execute.
- `tool/`: Go-based CLI tooling used within the repository, including tools for validating plans and specifications.
- `specs/` & `plans/`: Documents generated during the Spec-Driven Development process for complex features.
- `AGENTS.md`: Crucial instructions and architectural details for AI agents operating in this repository. Agents must read and update this file to share knowledge.
- `SOUL.md`: The core principles, goals, and "personality" constraints that guide the AI Factory's autonomous evolution.

## Architecture & Development Process

The AI Factory is designed as a set of Kubernetes-native workloads and operators. The default installer targets the active `kubectl` context; GKE-specific credential setup remains available by setting `KUBECONFIG_MODE=gke`.

For a generic Kubernetes cluster, select the target context first and provide a writable image registry prefix:

```bash
kubectl config use-context <cluster-context>
IMAGE_PREFIX=registry.example.com/ai-factory/ components/install
```

For a local or remote kind cluster, point `kubectl` at the cluster first, provide a tag that is already present in the kind nodes or can be loaded by `agent-sandbox`, and optionally reuse a local checkout of `agent-sandbox`:

```bash
export KUBECONFIG=/path/to/kubeconfig
kubectl cluster-info

IMAGE_PREFIX=localhost/ai-factory/ \
IMAGE_TAG=dev \
KIND_CLUSTER_NAME=fire-kind-cluster \
AGENT_SANDBOX_SRC=/path/to/agent-sandbox \
components/agent-sandbox/install
```

If the controller image was built and loaded into kind manually, skip the build step and only apply the manifests:

```bash
IMAGE_PREFIX=localhost/ai-factory/ \
IMAGE_TAG=dev \
AGENT_SANDBOX_SRC=/path/to/agent-sandbox \
AGENT_SANDBOX_BUILD_IMAGES=false \
components/agent-sandbox/install
```

After installation, run a small end-to-end sandbox check. By default this creates a warm pool and claim, copies this repository into the sandbox, and validates the `factory-runtime-proxy` spec from inside the sandbox:

```bash
DEV_IMAGE=golang:latest components/agent-sandbox/smoke-test
```

Use a mirror image if the cluster cannot pull directly from Docker Hub:

```bash
DEV_IMAGE=docker.m.daocloud.io/library/golang:1.26.4 components/agent-sandbox/smoke-test
```

For GKE, the installer can fetch credentials and derive a GCR image prefix from `gcloud`:

```bash
KUBECONFIG_MODE=gke CLUSTER_NAME=ai-factory ZONE=us-central1-a components/install
```

Optional GKE-oriented components such as `service-portals` are disabled by default and can be enabled with `INSTALL_SERVICE_PORTALS=true`.

The next control-plane layer is `FactoryTask`, a Kubernetes custom resource that captures one coding-agent task without coupling the task to a specific Git provider. A task can reference either GitHub or GitLab repositories through the same source contract:

```yaml
source:
  provider: github # or gitlab
  host: github.com
  repository: liyuerich/ai-factory
  baseRef: main
```

Install the CRD with:

```bash
components/factory-task/install
```

Validate or inspect a task locally with:

```bash
go run ./factory/cmd/factory task validate examples/factory-task-github.yaml
go run ./factory/cmd/factory task plan examples/factory-task-gitlab.yaml
```

Render the first controller output, a `SandboxClaim`, without touching a cluster:

```bash
go run ./factory/cmd/factory task controller manifest examples/factory-task-github.yaml
```

Run one task against the active `kubectl` context:

```bash
go run ./factory/cmd/factory task controller run-once examples/factory-task-github.yaml
```

The `run-once` command is the first controller slice: it applies the `FactoryTask`, patches task status through the task lifecycle, creates the `SandboxClaim`, waits for agent-sandbox to bind a sandbox, and executes the generated plan inside the sandbox container.
When `spec.work.instructions` is set, the plan runs the configured coding agent
inside the cloned repository before running `spec.work.commands`. The default
agent command is `gemini --yolo`; override it with `spec.agent.command`.

Patch task status directly when debugging a controller transition:

```bash
go run ./factory/cmd/factory task controller patch-status validate-ai-factory-spec \
  --phase Running \
  --reason ManualDebug \
  --message "debugging status patch"
```

Run the first long-running controller loop against the active `kubectl` context:

```bash
go run ./factory/cmd/factory task controller watch --namespace default
```

The watch controller polls `FactoryTask` resources, reconciles tasks whose phase is empty, `Pending`, `ClaimCreated`, or `SandboxReady`, creates the matching `SandboxClaim`, executes the generated plan, and patches status. Use `--once` for a single reconciliation pass and `--retry-failed` to retry failed tasks.

Deploy the long-running FactoryTask runtime into the active cluster with:

```bash
FACTORY_IMAGE=registry.example.com/ai-factory/factory:latest \
INSTALL_FACTORY_TASK_RUNTIME=true \
GITHUB_TOKEN=... \
GITLAB_TOKEN=... \
WEBHOOK_SECRET=... \
components/factory-task/install
```

This installs the watch controller, the GitHub/GitLab issue webhook service,
RBAC, and optional provider credentials. See
`components/factory-task/README.md` for runtime settings, optional Ingress,
webhook endpoints, and sandbox git authentication.

We follow a **Spec-Driven Development** process for complex features, handled entirely by interacting agents:
1. **Spec Generation:** The `speccer` agent generates specifications.
2. **Planning:** The `planner` agent creates detailed implementation plans.
3. **Execution:** The `builder` agent writes the code.
4. **Review:** The `reviewer` agent automatically reviews and approves pull requests.

Agents are triggered by GitHub events (e.g., assigning issues, requesting reviews).

## Contributing

This project is licensed under the [Apache 2.0 License](LICENSE).

**Note: We do not expect human contributions in this phase of the experiment.**

We follow [Google's Open Source Community Guidelines](https://opensource.google.com/conduct/).

## Disclaimer

This is not an officially supported Google product.

This project is not eligible for the Google Open Source Software Vulnerability Rewards Program.
