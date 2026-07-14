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

"""Validation for model responses that are about to be executed as Bash."""

import os
import re
import subprocess
import tempfile


SCRIPT_HEADER = "#!/usr/bin/env bash\nset -euo pipefail\n"


class ScriptValidationError(ValueError):
    """Raised when a model response is not safe executable shell content."""


_PROSE_PATTERNS = (
    re.compile(r"^no(?:\s+further)?\s+changes?\s+(?:are|were)?\s*(?:needed|required)\b", re.IGNORECASE),
    re.compile(r"^(?:the|this|these|those)\s+(?:implementation|change|task|work|code)\b", re.IGNORECASE),
    re.compile(r"^(?:i|we)\s+(?:have|has|did|updated|completed|fixed|implemented|verified)\b", re.IGNORECASE),
    re.compile(r"^(?:(?:all|the)\s+)?tests?\s+(?:pass|passed|are\s+passing)\b", re.IGNORECASE),
    re.compile(r"^(?:summary|status|result)\s*:", re.IGNORECASE),
    re.compile(r"^(?:done|completed|finished)\s*[.!]?$", re.IGNORECASE),
    re.compile(r"^(?:everything|nothing)\b.*\b(?:ready|needed|pass|passed|complete|done)\b", re.IGNORECASE),
    re.compile(r"^no\s+(?:further\s+)?action\b.*\bneeded\b", re.IGNORECASE),
    re.compile(r"^(?:run|execute|please)\b.*\b(?:test|check|change|implementation)\b", re.IGNORECASE),
    re.compile(r"^(?:implementation|changes?|task|work)\s+(?:is|are|was|were)\s+(?:complete|done|finished)\b", re.IGNORECASE),
)


def _first_code_line(script):
    for line in script.splitlines():
        stripped = line.strip()
        if stripped and not stripped.startswith("#"):
            return stripped
    return ""


def _looks_like_shell_construct(line):
    if line.startswith((
        "#!",
        "if ",
        "for ",
        "while ",
        "until ",
        "case ",
        "select ",
        "function ",
        "set ",
        "export ",
        "readonly ",
        "local ",
        "source ",
        ". ",
        "cd ",
        "command ",
        "eval ",
        "exec ",
        "trap ",
        "return ",
        "exit ",
        "unset ",
        "umask ",
        "alias ",
        "unalias ",
        "shift ",
        "getopts ",
        "true",
        "false",
        "[",
        "[[",
        "{",
        "(",
    )):
        return True
    if re.match(r"^[A-Za-z_][A-Za-z0-9_]*=", line):
        return True
    command = re.match(r"^[A-Za-z0-9_./+:-]+", line)
    if not command:
        return False
    token = command.group(0)
    if token.startswith(("/", "./", "../", "~/")):
        return True
    return token in {
        "awk",
        "bash",
        "cat",
        "chmod",
        "cp",
        "curl",
        "docker",
        "echo",
        "find",
        "git",
        "go",
        "grep",
        "head",
        "jq",
        "make",
        "mkdir",
        "mv",
        "node",
        "npm",
        "pip",
        "printf",
        "python",
        "python3",
        "rm",
        "rg",
        "sed",
        "sh",
        "sort",
        "tail",
        "tar",
        "test",
        "touch",
        "tr",
        "unzip",
        "wc",
        "which",
        "xargs",
    }


def _looks_like_natural_language(script):
    line = _first_code_line(script)
    if not line or _looks_like_shell_construct(line):
        return False
    if line.startswith(("- ", "* ")):
        return True
    if any(pattern.search(line) for pattern in _PROSE_PATTERNS):
        return True
    words = re.findall(r"[A-Za-z]+", line)
    prose_words = {"and", "are", "is", "of", "the", "to", "was", "were", "with"}
    return (
        len(words) >= 8
        and line.endswith((".", "!", "?"))
        and len(prose_words.intersection(word.lower() for word in words)) >= 3
    )


def _truncate(value, limit=2000):
    value = str(value)
    if len(value) <= limit:
        return value
    return value[:limit] + f"... <truncated {len(value) - limit} chars>"


def validate_shell_script(script):
    """Return script if it is executable shell content, otherwise raise."""
    script = (script or "").strip()
    if not script:
        raise ScriptValidationError("response was empty")
    if "```" in script:
        raise ScriptValidationError("Markdown code fences are not allowed")
    if _looks_like_natural_language(script):
        raise ScriptValidationError("response contained explanatory prose instead of a shell script")
    if not _first_code_line(script):
        raise ScriptValidationError("response contained no executable shell content")

    script_path = None
    try:
        with tempfile.NamedTemporaryFile("w", prefix="ai-factory-validate-", suffix=".sh", delete=False) as handle:
            handle.write(SCRIPT_HEADER)
            handle.write(script)
            handle.write("\n")
            script_path = handle.name
        completed = subprocess.run(
            ["/bin/bash", "-n", script_path],
            check=False,
            capture_output=True,
            text=True,
            timeout=10,
        )
    except subprocess.TimeoutExpired as exc:
        raise ScriptValidationError("shell syntax validation timed out") from exc
    finally:
        if script_path:
            try:
                os.unlink(script_path)
            except OSError:
                pass

    if completed.returncode != 0:
        detail = _truncate(completed.stderr or completed.stdout or "shell syntax validation failed")
        raise ScriptValidationError(f"shell syntax validation failed: {detail}")
    return script
