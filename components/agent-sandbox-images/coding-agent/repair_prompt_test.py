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

import unittest

from repair_prompt import (
    build_repair_prompt,
    detect_missing_command,
    format_failure_output,
)


class RepairPromptTest(unittest.TestCase):
    def test_command_unavailable_prompt_has_constraints_and_complete_output(self):
        failure_output = "\n".join(
            [
                "exit_code=127",
                "--- stdout ---",
                "attempting YAML validation",
                "--- stderr ---",
                "/tmp/ai-factory-agent.sh: line 3: yaml: command not found",
                "final diagnostic line",
            ]
        )

        prompt = build_repair_prompt(failure_output)

        self.assertEqual(detect_missing_command(failure_output), "yaml")
        self.assertIn("Missing command: `yaml`", prompt)
        self.assertIn("Do not invoke `yaml` again", prompt)
        self.assertIn("Do not install `yaml`", prompt)
        self.assertIn("Use `python3` with the installed PyYAML", prompt)
        self.assertIn("Known available Sandbox tools", prompt)
        self.assertIn(failure_output, prompt)
        self.assertTrue(prompt.endswith("final diagnostic line\n--- failure output end ---"))

    def test_detects_exec_style_missing_command(self):
        failure_output = 'exec: "terraform": executable file not found in $PATH'
        self.assertEqual(detect_missing_command(failure_output), "terraform")
        prompt = build_repair_prompt(failure_output)
        self.assertIn("Missing command: `terraform`", prompt)
        self.assertIn("using only the known available Sandbox tools", prompt)

    def test_complete_failure_output_is_not_truncated(self):
        stdout = "x" * 13000 + " stdout-end"
        stderr = "y" * 13000 + " stderr-end"
        failure_output = format_failure_output(127, stdout, stderr)
        prompt = build_repair_prompt(failure_output)

        self.assertIn(stdout, failure_output)
        self.assertIn(stderr, failure_output)
        self.assertIn(failure_output, prompt)

    def test_unrelated_failure_preserves_output_without_false_tool_diagnosis(self):
        failure_output = "go test ./...: exit status 1\nFAIL example.test"
        prompt = build_repair_prompt(failure_output)

        self.assertEqual(detect_missing_command(failure_output), "")
        self.assertNotIn("Sandbox command unavailable", prompt)
        self.assertNotIn("Missing command:", prompt)
        self.assertIn(failure_output, prompt)

        user_log = "Please install go: command not found is not an executable failure"
        self.assertEqual(detect_missing_command(user_log), "")

    def test_prompt_is_provider_neutral(self):
        prompt = build_repair_prompt("/tmp/script.sh: line 4: yq: command not found")
        self.assertNotIn("openai", prompt.lower())
        self.assertNotIn("codex", prompt.lower())

    def test_prompt_includes_previous_script_and_prior_diagnostics(self):
        previous_script = "printf '%s\\n' previous"
        prior_attempts = "round 1 returned an empty repair script"
        failure_output = "exit_code=1\n--- stderr ---\nfocused failure"

        prompt = build_repair_prompt(
            failure_output,
            previous_script=previous_script,
            prior_attempts=prior_attempts,
        )

        self.assertIn(previous_script, prompt)
        self.assertIn(prior_attempts, prompt)
        self.assertIn(failure_output, prompt)
        self.assertIn("must start with a shell command or a shebang", prompt)
        self.assertIn("must not contain tool calls", prompt)
        self.assertIn("do not derive the repository root", prompt)
        self.assertIn("Do not run `python3 -m py_compile`", prompt)


if __name__ == "__main__":
    unittest.main()
