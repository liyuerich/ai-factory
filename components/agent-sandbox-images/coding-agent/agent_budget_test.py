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

import json
import unittest

from agent_budget import (
    AgentBudgetError,
    ExecutionDeadline,
    PhaseRoundBudget,
    post_chat_completion,
)


class FakeClock:
    def __init__(self):
        self.now = 0.0

    def __call__(self):
        return self.now

    def sleep(self, seconds):
        self.now += seconds


class FakeResponse:
    def __init__(self, payload):
        self.payload = json.dumps(payload).encode("utf-8")

    def __enter__(self):
        return self

    def __exit__(self, _type, _value, _traceback):
        return False

    def read(self):
        return self.payload


class AgentBudgetTest(unittest.TestCase):
    def test_tool_round_exhaustion_identifies_phase_and_limit(self):
        budget = PhaseRoundBudget(
            "tool-exploration",
            2,
            "ToolRoundsExhausted",
        )

        self.assertEqual(budget.next_round(), 1)
        self.assertEqual(budget.next_round(), 2)
        with self.assertRaises(AgentBudgetError) as raised:
            budget.next_round()

        self.assertEqual(raised.exception.reason, "ToolRoundsExhausted")
        self.assertEqual(raised.exception.phase, "tool-exploration")
        self.assertIn("used_rounds=2; limit=2", str(raised.exception))

    def test_request_timeout_is_bounded_and_phase_aware(self):
        clock = FakeClock()
        deadline = ExecutionDeadline(30, clock=clock)
        observed_timeouts = []

        def timeout_opener(_request, timeout):
            observed_timeouts.append(timeout)
            clock.now += timeout
            raise TimeoutError("provider did not respond")

        with self.assertRaises(AgentBudgetError) as raised:
            post_chat_completion(
                {"model": "test"},
                "https://provider.example/v1",
                "secret",
                "final-script",
                5,
                deadline,
                max_attempts=2,
                opener=timeout_opener,
                sleeper=clock.sleep,
            )

        self.assertEqual(observed_timeouts, [5, 5])
        self.assertEqual(raised.exception.reason, "ModelRequestTimeout")
        self.assertEqual(raised.exception.phase, "final-script")
        self.assertIn("attempts=2/2", str(raised.exception))
        self.assertIn("request_timeout_seconds=5.000", str(raised.exception))

    def test_request_timeout_is_capped_by_total_deadline(self):
        clock = FakeClock()
        deadline = ExecutionDeadline(3, clock=clock)
        observed_timeouts = []

        def successful_opener(_request, timeout):
            observed_timeouts.append(timeout)
            return FakeResponse({"choices": [{"message": {"content": "true"}}]})

        payload = post_chat_completion(
            {"model": "test"},
            "https://provider.example/v1",
            "secret",
            "repair-script",
            90,
            deadline,
            opener=successful_opener,
        )

        self.assertEqual(payload["choices"][0]["message"]["content"], "true")
        self.assertEqual(observed_timeouts, [3])

    def test_invalid_provider_response_is_preserved(self):
        clock = FakeClock()
        deadline = ExecutionDeadline(10, clock=clock)

        class InvalidResponse(FakeResponse):
            def __init__(self):
                self.payload = b"provider returned plain text"

        with self.assertRaises(AgentBudgetError) as raised:
            post_chat_completion(
                {"model": "test"},
                "https://provider.example/v1",
                "secret",
                "tool-exploration",
                5,
                deadline,
                opener=lambda _request, timeout: InvalidResponse(),
            )

        self.assertEqual(raised.exception.reason, "InvalidModelResponse")
        self.assertEqual(
            raised.exception.diagnostics,
            "provider returned plain text",
        )


if __name__ == "__main__":
    unittest.main()
