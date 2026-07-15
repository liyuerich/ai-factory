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

"""Provider-neutral repair prompts for coding-agent script failures."""

import re


SANDBOX_TOOL_GUIDANCE = """Known available Sandbox tools:
- Shell/core tools: bash, sh, awk, cat, cp, find, grep, head, mkdir, mv, rm, sed, sort, tail, tar, touch, tr, unzip, wc, and xargs.
- Development tools: git, go, make, node, npm, python3, pip3, rg, jq, and curl.
- Python includes the PyYAML module (`import yaml`), but there is no `yaml` or `yq` command.
Do not assume commands outside this list are installed. Do not install packages during a repair; rewrite the script with available tools instead."""


_MISSING_COMMAND_PATTERNS = (
    re.compile(
        r"(?m)^[^\n]*:\s*line\s+\d+:\s*([A-Za-z0-9_.+-]+):\s*command not found\s*$"
    ),
    re.compile(r'exec:\s*"([^"]+)":\s*executable file not found'),
    re.compile(
        r"(?m)^(?:/bin/)?(?:ba)?sh:\s*\d+:\s*([A-Za-z0-9_.+-]+):\s*not found\s*$"
    ),
)

_COMMAND_ALTERNATIVES = {
    "ag": "Use `rg` or `grep` for text search.",
    "apply_patch": "Use `python3` or `sed` for a small, focused file edit.",
    "fd": "Use `find` for file discovery.",
    "python": "Use `python3`.",
    "wget": "Use `curl`.",
    "yaml": "Use `python3` with the installed PyYAML module (`import yaml`).",
    "yq": "Use `python3` with the installed PyYAML module (`import yaml`).",
}


def detect_missing_command(failure_output):
    """Return the unavailable command named by common shell/exec errors."""
    failure_output = str(failure_output or "")
    for pattern in _MISSING_COMMAND_PATTERNS:
        match = pattern.search(failure_output)
        if match:
            return match.group(1)
    return ""


def recommended_alternative(command):
    """Return an actionable alternative that is available in the Sandbox."""
    return _COMMAND_ALTERNATIVES.get(
        command,
        "Rewrite the failed step using only the known available Sandbox tools. "
        "If no equivalent exists, fail clearly instead of installing or assuming the command.",
    )


def format_failure_output(returncode, stdout="", stderr=""):
    """Format complete command output without dropping or truncating diagnostics."""
    return "\n".join(
        [
            f"exit_code={returncode}",
            "--- stdout ---",
            stdout or "",
            "--- stderr ---",
            stderr or "",
        ]
    )


def build_repair_prompt(failure_output, previous_script="", prior_attempts=""):
    """Build a repair request while preserving the complete redacted failure."""
    failure_output = str(failure_output or "")
    previous_script = str(previous_script or "")
    prior_attempts = str(prior_attempts or "")
    parts = [
        "The shell script you generated failed when ai-factory executed it.",
        "The repository is left in its current modified state.",
        "Return only a concise POSIX shell script that repairs the current state and reruns the relevant checks.",
        "Do not explain, do not use Markdown, do not commit, and do not push.",
        "The response must start with a shell command or a shebang and must not contain tool calls.",
    ]

    missing_command = detect_missing_command(failure_output)
    if missing_command:
        parts.extend(
            [
                "",
                "Sandbox command unavailable:",
                f"- Missing command: `{missing_command}`.",
                f"- Do not invoke `{missing_command}` again in the repair script.",
                f"- Do not install `{missing_command}` or assume it exists in the Sandbox.",
                f"- Recommended alternative: {recommended_alternative(missing_command)}",
                "",
                SANDBOX_TOOL_GUIDANCE,
            ]
        )

    if previous_script:
        parts.extend(
            [
                "",
                "Previous failed script (do not return this script unchanged):",
                "--- previous script start ---",
                previous_script,
                "--- previous script end ---",
            ]
        )

    if prior_attempts:
        parts.extend(
            [
                "",
                "Earlier repair history (do not repeat these scripts or invalid formats):",
                "--- repair history start ---",
                prior_attempts,
                "--- repair history end ---",
            ]
        )

    parts.extend(
        [
            "",
            "Complete failure output (verbatim, with secrets already redacted):",
            "--- failure output start ---",
            failure_output,
            "--- failure output end ---",
        ]
    )
    return "\n".join(parts)
