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

from script_validation import ScriptValidationError, validate_shell_script


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

    def test_rejects_malformed_shell(self):
        self.assertInvalid("if true; then\n  printf '%s\\n' 'missing fi'", "syntax validation failed")

    def test_accepts_valid_shell(self):
        script = "set -e\nprintf '%s\\n' 'valid'"
        self.assertEqual(validate_shell_script(script), script)

    def test_accepts_shell_comments_before_command(self):
        script = "# explain the focused change\nprintf '%s\\n' 'valid'"
        self.assertEqual(validate_shell_script(script), script)


if __name__ == "__main__":
    unittest.main()
