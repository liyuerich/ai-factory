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

"""Bounded repair-loop state and duplicate-attempt protection."""

from dataclasses import dataclass
import hashlib

from agent_budget import AgentBudgetError
from repair_prompt import build_repair_prompt, format_failure_output


@dataclass(frozen=True)
class RepairCandidate:
    """One model response produced for a repair round."""

    script: str = ""
    error: str = ""
    diagnostics: str = ""


@dataclass(frozen=True)
class FailedScriptAttempt:
    """A generated or repair script that was executed and failed."""

    label: str
    script: str
    failure_output: str
    signature: str


@dataclass(frozen=True)
class InvalidResponseAttempt:
    """A repair response that did not contain an executable script."""

    round_number: int
    reason: str
    diagnostics: str
    signature: str


class RepairLoopTerminated(RuntimeError):
    """Raised with complete diagnostics when repair cannot continue safely."""

    def __init__(self, reason, returncode, diagnostics):
        super().__init__(reason)
        self.reason = reason
        self.returncode = returncode
        self.diagnostics = diagnostics


def normalize_script(script):
    """Normalize inconsequential whitespace before identifying a script."""
    lines = str(script or "").replace("\r\n", "\n").replace("\r", "\n").splitlines()
    return "\n".join(line.rstrip() for line in lines).strip()


def script_signature(script):
    """Return a stable identity for a generated shell script."""
    return hashlib.sha256(normalize_script(script).encode("utf-8")).hexdigest()


def response_format_signature(reason):
    """Identify repeated invalid response formats independently of payload text."""
    normalized = " ".join(str(reason or "").lower().split())
    return hashlib.sha256(normalized.encode("utf-8")).hexdigest()


class RepairHistory:
    """Track failed scripts and invalid model response formats."""

    def __init__(self):
        self.failed_scripts = []
        self.invalid_responses = []
        self._failed_script_signatures = set()
        self._invalid_response_signatures = set()

    def has_failed_script(self, script):
        return script_signature(script) in self._failed_script_signatures

    def record_failed_script(self, label, script, failure_output):
        signature = script_signature(script)
        attempt = FailedScriptAttempt(
            label=str(label),
            script=str(script or ""),
            failure_output=str(failure_output or ""),
            signature=signature,
        )
        self.failed_scripts.append(attempt)
        self._failed_script_signatures.add(signature)
        return attempt

    def record_invalid_response(self, round_number, reason, diagnostics):
        signature = response_format_signature(reason)
        repeated = signature in self._invalid_response_signatures
        self.invalid_responses.append(
            InvalidResponseAttempt(
                round_number=round_number,
                reason=str(reason or ""),
                diagnostics=str(diagnostics or ""),
                signature=signature,
            )
        )
        self._invalid_response_signatures.add(signature)
        return repeated

    def prompt_history(self):
        """Return complete earlier diagnostics for the next repair request."""
        parts = []
        for attempt in self.failed_scripts[:-1]:
            parts.extend(
                [
                    f"Failed script {attempt.label} (sha256={attempt.signature}):",
                    "--- prior script start ---",
                    attempt.script,
                    "--- prior script end ---",
                    "--- prior failure output start ---",
                    attempt.failure_output,
                    "--- prior failure output end ---",
                ]
            )
        for attempt in self.invalid_responses:
            parts.extend(
                [
                    f"Invalid repair response round {attempt.round_number} "
                    f"(sha256={attempt.signature}): {attempt.reason}",
                    "--- invalid response diagnostics start ---",
                    attempt.diagnostics,
                    "--- invalid response diagnostics end ---",
                ]
            )
        return "\n".join(parts)

    def terminal_diagnostics(self, reason, max_rounds, candidate_script=""):
        """Return a complete final report for the FactoryTask failure."""
        parts = [
            f"Repair loop terminated: {reason}",
            f"Configured maximum repair rounds: {max_rounds}",
        ]
        if candidate_script:
            parts.extend(
                [
                    f"Rejected candidate script sha256={script_signature(candidate_script)}:",
                    "--- rejected candidate script start ---",
                    candidate_script,
                    "--- rejected candidate script end ---",
                ]
            )
        for attempt in self.failed_scripts:
            parts.extend(
                [
                    f"Failed script {attempt.label} (sha256={attempt.signature}):",
                    "--- failed script start ---",
                    attempt.script,
                    "--- failed script end ---",
                    "--- failure output start ---",
                    attempt.failure_output,
                    "--- failure output end ---",
                ]
            )
        for attempt in self.invalid_responses:
            parts.extend(
                [
                    f"Invalid repair response round {attempt.round_number} "
                    f"(sha256={attempt.signature}): {attempt.reason}",
                    "--- invalid response diagnostics start ---",
                    attempt.diagnostics,
                    "--- invalid response diagnostics end ---",
                ]
            )
        return "\n".join(parts)


def _failure_output(completed, redact):
    return redact(
        format_failure_output(
            completed.returncode,
            completed.stdout,
            completed.stderr,
        )
    )


def run_repair_loop(
    initial_script,
    initial_completed,
    max_rounds,
    request_repair,
    execute_script,
    redact=lambda value: str(value),
):
    """Run bounded repairs, rejecting duplicate scripts before execution.

    request_repair receives ``(round_number, max_rounds, prompt)`` and returns
    a RepairCandidate. execute_script receives ``(script, label)`` and returns
    an object compatible with subprocess.CompletedProcess.
    """
    if initial_completed.returncode == 0:
        return initial_completed

    history = RepairHistory()
    history.record_failed_script(
        "generated",
        redact(initial_script),
        _failure_output(initial_completed, redact),
    )
    completed = initial_completed

    for round_number in range(1, max_rounds + 1):
        previous = history.failed_scripts[-1]
        prompt = build_repair_prompt(
            previous.failure_output,
            previous_script=previous.script,
            prior_attempts=history.prompt_history(),
        )
        try:
            candidate = request_repair(round_number, max_rounds, prompt)
        except AgentBudgetError as exc:
            terminal_reason = str(exc)
            diagnostics = history.terminal_diagnostics(
                terminal_reason,
                max_rounds,
            )
            if exc.diagnostics:
                diagnostics = "\n".join(
                    [
                        diagnostics,
                        "--- provider diagnostics start ---",
                        redact(exc.diagnostics),
                        "--- provider diagnostics end ---",
                    ]
                )
            raise RepairLoopTerminated(
                terminal_reason,
                1,
                diagnostics,
            ) from exc
        if candidate.error or not candidate.script.strip():
            reason = candidate.error or "model returned an empty repair script"
            repeated = history.record_invalid_response(
                round_number,
                reason,
                redact(candidate.diagnostics),
            )
            if repeated:
                terminal_reason = (
                    "RepeatedInvalidRepairResponseFormat: the model returned "
                    f"the same invalid format again ({reason})"
                )
                raise RepairLoopTerminated(
                    terminal_reason,
                    1,
                    history.terminal_diagnostics(terminal_reason, max_rounds),
                )
            continue

        display_script = redact(candidate.script)
        if history.has_failed_script(display_script):
            terminal_reason = (
                "RepeatedInvalidRepairScript: candidate matches a previously "
                f"failed script (sha256={script_signature(display_script)})"
            )
            raise RepairLoopTerminated(
                terminal_reason,
                1,
                history.terminal_diagnostics(
                    terminal_reason,
                    max_rounds,
                    candidate_script=display_script,
                ),
            )

        completed = execute_script(candidate.script, f"repair {round_number}")
        if completed.returncode == 0:
            return completed
        history.record_failed_script(
            f"repair {round_number}",
            display_script,
            _failure_output(completed, redact),
        )

    terminal_reason = (
        f"RepairRoundsExhausted: no repair succeeded within {max_rounds} rounds"
    )
    raise RepairLoopTerminated(
        terminal_reason,
        completed.returncode or 1,
        history.terminal_diagnostics(terminal_reason, max_rounds),
    )
