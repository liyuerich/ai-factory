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


class RepairResponseError(ValueError):
    """Raised when a model repair response does not contain a usable script."""


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

_PYTHON_HEREDOC = re.compile(
    r"^\s*(?:command\s+)?(?:[^\s]+/)?python(?:3(?:\.\d+)?)?\b.*?"
    r"(?P<operator><<-?)\s*(?P<quote>['\"])(?P<delimiter>[A-Za-z_][A-Za-z0-9_]*)"
    r"(?P=quote)\s*$"
)

_STANDALONE_SHELL_FENCE = re.compile(
    r"```(?:bash|sh|shell)[ \t]*\n(?P<script>.*?)\n```",
    re.IGNORECASE | re.DOTALL,
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


def normalize_model_script(script):
    """Unwrap one standalone Shell fence without extracting mixed Markdown."""
    script = str(script or "").replace("\r\n", "\n").replace("\r", "\n").strip()
    if "```" not in script:
        return script

    match = _STANDALONE_SHELL_FENCE.fullmatch(script)
    if not match or "```" in match.group("script"):
        raise ScriptValidationError(
            "Markdown code fences must be one standalone bash, sh, or shell block"
        )
    return match.group("script").strip()


def extract_repair_script(payload):
    """Return script content from a repair response or raise a stable error."""
    try:
        choice = payload["choices"][0]
        message = choice["message"]
    except (KeyError, IndexError, TypeError) as exc:
        raise RepairResponseError(
            f"repair response did not contain choices[0].message: {exc}"
        ) from exc

    content = message.get("content") or ""
    finish_reason = choice.get("finish_reason") or ""
    tool_calls = message.get("tool_calls") or []
    if finish_reason == "length":
        raise RepairResponseError("repair script was truncated by the token limit")
    if finish_reason == "tool_calls" or (tool_calls and not content):
        raise RepairResponseError("model returned an empty repair response with tool calls")
    if not content.strip():
        raise RepairResponseError("model returned an empty repair script")
    return content


def _heredoc_declarations(line):
    """Return (delimiter, strip_tabs) pairs for unquoted shell heredocs."""
    declarations = []
    index = 0
    quote = None
    while index < len(line):
        char = line[index]
        if quote:
            if char == "\\" and quote == '"':
                index += 2
                continue
            if char == quote:
                quote = None
            index += 1
            continue
        if char in ("'", '"'):
            quote = char
            index += 1
            continue
        if char == "\\":
            index += 2
            continue
        if char == "#" and (index == 0 or line[index - 1].isspace()):
            break
        if not line.startswith("<<", index) or line.startswith("<<<", index):
            index += 1
            continue

        cursor = index + 2
        strip_tabs = cursor < len(line) and line[cursor] == "-"
        if strip_tabs:
            cursor += 1
        while cursor < len(line) and line[cursor].isspace():
            cursor += 1

        delimiter_quote = line[cursor] if cursor < len(line) and line[cursor] in ("'", '"') else None
        if delimiter_quote:
            cursor += 1
        delimiter_start = cursor
        while cursor < len(line) and (line[cursor].isalnum() or line[cursor] == "_"):
            cursor += 1
        delimiter = line[delimiter_start:cursor]
        if delimiter_quote:
            if cursor >= len(line) or line[cursor] != delimiter_quote:
                index += 2
                continue
            cursor += 1
        if delimiter and (delimiter[0].isalpha() or delimiter[0] == "_"):
            declarations.append((delimiter, strip_tabs))
            index = cursor
            continue
        index += 2
    return declarations


def _validate_heredoc_terminators(script):
    """Reject heredocs whose terminator is missing from the shell script."""
    lines = script.splitlines()
    index = 0
    while index < len(lines):
        declarations = _heredoc_declarations(lines[index])
        if not declarations:
            index += 1
            continue

        body_index = index + 1
        for delimiter, strip_tabs in declarations:
            while body_index < len(lines):
                candidate = lines[body_index].lstrip("\t") if strip_tabs else lines[body_index]
                if candidate == delimiter:
                    break
                body_index += 1
            if body_index >= len(lines):
                raise ScriptValidationError(
                    f"shell syntax validation failed: unterminated heredoc {delimiter!r}"
                )
            body_index += 1
        index = body_index


def _validate_embedded_python_heredocs(script):
    """Compile literal Python heredocs before the surrounding shell executes."""
    lines = script.splitlines()
    index = 0
    while index < len(lines):
        match = _PYTHON_HEREDOC.match(lines[index])
        if not match:
            index += 1
            continue

        delimiter = match.group("delimiter")
        strip_tabs = match.group("operator") == "<<-"
        body = []
        index += 1
        while index < len(lines):
            candidate = lines[index].lstrip("\t") if strip_tabs else lines[index]
            if candidate == delimiter:
                break
            body.append(lines[index].lstrip("\t") if strip_tabs else lines[index])
            index += 1

        # bash -n reports an unterminated heredoc. Leave that diagnostic to the
        # shell validation path below instead of replacing it with a Python one.
        if index >= len(lines):
            return

        try:
            compile("\n".join(body) + "\n", "<generated Python heredoc>", "exec")
        except SyntaxError as exc:
            location = f"line {exc.lineno}" if exc.lineno else "unknown line"
            raise ScriptValidationError(
                f"embedded Python syntax validation failed at {location}: {exc.msg}"
            ) from exc
        index += 1


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

    _validate_heredoc_terminators(script)

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

    shell_diagnostic = completed.stderr or completed.stdout or ""
    unterminated_heredoc = (
        "here-document at line" in shell_diagnostic
        and "delimited by end-of-file" in shell_diagnostic
    )
    if completed.returncode != 0 or unterminated_heredoc:
        detail = _truncate(completed.stderr or completed.stdout or "shell syntax validation failed")
        raise ScriptValidationError(f"shell syntax validation failed: {detail}")
    _validate_embedded_python_heredocs(script)
    return script
