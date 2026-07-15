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

import tempfile
import unittest
from pathlib import Path

from script_validation import (
    RepairResponseError,
    ScriptValidationError,
    extract_repair_script,
    normalize_model_script,
    validate_shell_script,
)


class ScriptValidationTest(unittest.TestCase):
    def assertInvalid(self, script, message):
        with self.assertRaisesRegex(ScriptValidationError, message):
            validate_shell_script(script)

    def test_rejects_natural_language_completion(self):
        self.assertInvalid(
            "No further changes are needed; the implementation satisfies the issue requirements and the requested tests pass.",
            "explanatory prose",
        )

    def test_rejects_generic_status_prose(self):
        self.assertInvalid("Everything is ready and all requested checks passed.", "explanatory prose")

    def test_rejects_markdown_fence(self):
        self.assertInvalid("```bash\necho done\n```", "code fences")

    def test_normalizes_one_standalone_shell_fence(self):
        for language in ("bash", "sh", "shell", "BASH"):
            with self.subTest(language=language):
                self.assertEqual(
                    normalize_model_script(
                        f"```{language}\r\nprintf '%s\\n' valid\r\n```\r\n"
                    ),
                    "printf '%s\\n' valid",
                )

    def test_preserves_unfenced_script(self):
        script = "printf '%s\\n' valid"
        self.assertEqual(normalize_model_script(script), script)

    def test_rejects_fence_with_surrounding_prose(self):
        with self.assertRaisesRegex(ScriptValidationError, "one standalone"):
            normalize_model_script("Here is the script:\n```bash\nprintf ok\n```")

    def test_rejects_multiple_or_unterminated_fences(self):
        for script in (
            "```bash\nprintf first\n```\n```sh\nprintf second\n```",
            "```bash\nprintf missing-end",
            "```python\nprint('not shell')\n```",
        ):
            with self.subTest(script=script):
                with self.assertRaisesRegex(ScriptValidationError, "one standalone"):
                    normalize_model_script(script)

    def test_rejects_malformed_shell(self):
        self.assertInvalid("if true; then\n  printf '%s\\n' 'missing fi'", "syntax validation failed")

    def test_rejects_unterminated_shell_heredoc(self):
        self.assertInvalid("cat <<'EOF'\nmissing terminator", "syntax validation failed")

    def test_rejects_unterminated_python_triple_quote(self):
        script = "\n".join(
            [
                "python3 - <<'PY'",
                'value = """unterminated',
                "PY",
            ]
        )
        self.assertInvalid(script, "embedded Python syntax validation failed")

    def test_invalid_script_is_rejected_before_execution(self):
        with tempfile.TemporaryDirectory() as directory:
            marker = Path(directory) / "executed"
            script = f"touch '{marker}'\nif true; then\n  echo missing-fi"
            self.assertInvalid(script, "syntax validation failed")
            self.assertFalse(marker.exists())

    def test_accepts_literal_regex_in_quoted_shell_input(self):
        script = r"printf '%s\n' '[\s\S]*?'"
        self.assertEqual(validate_shell_script(script), script)

    def test_accepts_heredoc_text_inside_quotes_and_comments(self):
        script = "\n".join(
            [
                "# fixture example: cat <<'EOF'",
                'printf \'%s\\n\' "cat <<EOF"',
            ]
        )
        self.assertEqual(validate_shell_script(script), script)

    def test_accepts_valid_shell(self):
        script = "set -e\nprintf '%s\\n' 'valid'"
        self.assertEqual(validate_shell_script(script), script)

    def test_accepts_shell_comments_before_command(self):
        script = "# explain the focused change\nprintf '%s\\n' 'valid'"
        self.assertEqual(validate_shell_script(script), script)

    def test_extracts_repair_script(self):
        payload = {
            "choices": [
                {
                    "finish_reason": "stop",
                    "message": {"content": "printf '%s\\n' repaired"},
                }
            ]
        }
        self.assertEqual(extract_repair_script(payload), "printf '%s\\n' repaired")

    def test_rejects_empty_repair_response(self):
        payload = {"choices": [{"finish_reason": "stop", "message": {"content": ""}}]}
        with self.assertRaisesRegex(RepairResponseError, "empty repair script"):
            extract_repair_script(payload)

    def test_rejects_empty_repair_response_with_tool_calls(self):
        payload = {
            "choices": [
                {
                    "finish_reason": "tool_calls",
                    "message": {
                        "content": "",
                        "tool_calls": [{"id": "call-1", "function": {"name": "Shell"}}],
                    },
                }
            ]
        }
        with self.assertRaisesRegex(RepairResponseError, "empty repair response with tool calls"):
            extract_repair_script(payload)


if __name__ == "__main__":
    unittest.main()
