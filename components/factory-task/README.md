# FactoryTask runtime

`components/factory-task/install` always installs the `FactoryTask` CRD. Set
`INSTALL_FACTORY_TASK_RUNTIME=true` to also deploy the runtime controllers:

- `factory-task-watch-controller`: watches `FactoryTask` resources and executes
  sandbox work.
- `factory-task-webhook`: receives GitHub/GitLab issue webhooks and creates
  `FactoryTask` resources.

## Install

```bash
FACTORY_IMAGE=registry.example.com/ai-factory/factory:latest \
INSTALL_FACTORY_TASK_RUNTIME=true \
GITHUB_TOKEN=... \
GITLAB_TOKEN=... \
WEBHOOK_SECRET=... \
components/factory-task/install
```

The `FACTORY_IMAGE` image must contain the `factory` CLI and `kubectl`.
Provider credentials are written to the `factory-task-secrets` secret only when
`GITHUB_TOKEN`, `GITLAB_TOKEN`, or `WEBHOOK_SECRET` is provided. Re-running the
installer without those values leaves any existing secret untouched.

## Runtime settings

| Variable | Default | Purpose |
| --- | --- | --- |
| `FACTORY_IMAGE` | required | Image used by the webhook and watch controller deployments. |
| `FACTORY_NAMESPACE` | `factory-system` | Namespace for runtime deployments, service account, service, and secret. |
| `FACTORY_TASK_NAMESPACE` | `default` | Namespace where webhook-created tasks are applied and watched. |
| `IMAGE_PULL_POLICY` | `IfNotPresent` | Runtime deployment image pull policy. |
| `WATCH_INTERVAL` | `15s` | FactoryTask polling interval. |
| `SANDBOX_TIMEOUT` | `5m` | SandboxClaim readiness timeout. |
| `REQUIRE_LABEL` | `ai-factory` | Issue label required to trigger tasks. |
| `SANDBOX_TEMPLATE` | `go-dev` | Sandbox template used for generated tasks. |
| `AGENT_NAME` | `builder` | Agent name used for generated tasks. |
| `TASK_COMMAND` | `go test ./...` | Command inserted into generated FactoryTasks. |
| `GITHUB_TOKEN` | empty | Token used for GitHub issue comments and pull requests. |
| `GITLAB_TOKEN` | empty | Token used for GitLab issue comments and merge requests. |
| `WEBHOOK_SECRET` | empty | GitHub webhook secret or GitLab webhook token. |

## Webhook endpoints

The runtime creates a ClusterIP service named `factory-task-webhook`.

For local testing:

```bash
kubectl -n factory-system port-forward svc/factory-task-webhook 8080:80
```

Use these paths in provider webhook settings:

- GitHub: `http://<host>/webhook/github`
- GitLab: `http://<host>/webhook/gitlab`

GitHub should send issue events with `X-Hub-Signature-256`. GitLab should send
issue events with `X-Gitlab-Token`.

## Permissions

The runtime service account can:

- create and patch `FactoryTask` resources
- create and watch `SandboxClaim` resources
- read pods
- execute commands in pods

Keep provider tokens scoped tightly. GitHub needs issue comment and pull request
permissions for the target repository. GitLab needs issue note and merge request
permissions for the target project.
