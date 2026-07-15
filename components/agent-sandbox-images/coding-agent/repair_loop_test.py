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

import subprocess
import unittest

from repair_loop import (
    RepairCandidate,
    RepairLoopTerminated,
    normalize_script,
    run_repair_loop,
    script_signature,
)


def completed(returncode, stdout="", stderr=""):
    return subprocess.CompletedProcess(
        args=[],
        returncode=returncode,
        stdout=stdout,
        stderr=stderr,
    )


class RepairLoopTest(unittest.TestCase):
    def test_script_signature_normalizes_line_endings_and_trailing_space(self):
        first = "printf ok  \r\n"
        second = "printf ok\n"
        self.assertEqual(normalize_script(first), normalize_script(second))
        self.assertEqual(script_signature(first), script_signature(second))

    def test_duplicate_failed_script_is_not_executed_twice(self):
        candidates = iter(
            [
                RepairCandidate(script="printf repair"),
                RepairCandidate(script="printf repair  \n"),
            ]
        )
        executed = []

        def request_repair(_round_number, _max_rounds, _prompt):
            return next(candidates)

        def execute_script(script, _label):
            executed.append(script)
            return completed(1, stderr="repair still failed")

        with self.assertRaises(RepairLoopTerminated) as raised:
            run_repair_loop(
                "printf generated",
                completed(1, stderr="generated failed"),
                3,
                request_repair,
                execute_script,
            )

        self.assertEqual(executed, ["printf repair"])
        self.assertIn("RepeatedInvalidRepairScript", raised.exception.reason)
        self.assertIn("repair still failed", raised.exception.diagnostics)

    def test_changed_repair_scripts_can_progress_to_success(self):
        candidates = iter(
            [
                RepairCandidate(script="printf first"),
                RepairCandidate(script="printf second"),
            ]
        )
        prompts = []
        executed = []

        def request_repair(_round_number, _max_rounds, prompt):
            prompts.append(prompt)
            return next(candidates)

        def execute_script(script, _label):
            executed.append(script)
            if script == "printf first":
                return completed(1, stderr="first repair failed")
            return completed(0, stdout="second repair passed")

        result = run_repair_loop(
            "printf generated",
            completed(1, stderr="generated failed"),
            3,
            request_repair,
            execute_script,
        )

        self.assertEqual(result.returncode, 0)
        self.assertEqual(executed, ["printf first", "printf second"])
        self.assertIn("printf generated", prompts[0])
        self.assertIn("generated failed", prompts[0])
        self.assertIn("printf first", prompts[1])
        self.assertIn("first repair failed", prompts[1])
        self.assertIn("Failed script generated", prompts[1])

    def test_repeated_invalid_response_format_stops_with_diagnostics(self):
        candidates = iter(
            [
                RepairCandidate(
                    error="model returned an empty repair script",
                    diagnostics="empty response one",
                ),
                RepairCandidate(
                    error="model returned an empty repair script",
                    diagnostics="empty response two",
                ),
            ]
        )
        executed = []

        with self.assertRaises(RepairLoopTerminated) as raised:
            run_repair_loop(
                "printf generated",
                completed(1, stderr="generated failed"),
                3,
                lambda *_args: next(candidates),
                lambda script, _label: executed.append(script),
            )

        self.assertEqual(executed, [])
        self.assertIn("RepeatedInvalidRepairResponseFormat", raised.exception.reason)
        self.assertIn("empty response one", raised.exception.diagnostics)
        self.assertIn("empty response two", raised.exception.diagnostics)

    def test_maximum_repair_rounds_are_respected(self):
        candidates = iter(
            [
                RepairCandidate(script="printf first"),
                RepairCandidate(script="printf second"),
            ]
        )
        executed = []

        def execute_script(script, _label):
            executed.append(script)
            return completed(1, stderr=f"{script} failed")

        with self.assertRaises(RepairLoopTerminated) as raised:
            run_repair_loop(
                "printf generated",
                completed(1, stderr="generated failed"),
                2,
                lambda *_args: next(candidates),
                execute_script,
            )

        self.assertEqual(executed, ["printf first", "printf second"])
        self.assertIn("RepairRoundsExhausted", raised.exception.reason)
        self.assertIn("Configured maximum repair rounds: 2", raised.exception.diagnostics)
        self.assertIn("printf first failed", raised.exception.diagnostics)
        self.assertIn("printf second failed", raised.exception.diagnostics)


if __name__ == "__main__":
    unittest.main()
