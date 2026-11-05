import unittest

from config import TrackerConfig
from request_policy import RequestGate


class FakeClock:
    def __init__(self):
        self.value = 0.0

    def now(self):
        return self.value

    def sleep(self, seconds):
        self.value += seconds


class RequestPolicyTests(unittest.TestCase):
    def test_rate_and_client_configuration_is_validated(self):
        config = TrackerConfig.from_dict({
            "max_requests_per_minute": 2,
            "max_price_checks_per_run": 30,
            "block_cooldown_minutes": 60,
            "http_client": "cloudscraper",
        })
        self.assertEqual(config.http_client, "cloudscraper")
        self.assertEqual(config.max_price_checks_per_run, 30)
        with self.assertRaisesRegex(ValueError, "max_requests_per_minute"):
            TrackerConfig.from_dict({"max_requests_per_minute": 0})
        with self.assertRaisesRegex(ValueError, "http_client"):
            TrackerConfig.from_dict({"http_client": "unknown"})

    def test_requests_are_spaced_and_repeated_blocks_open_circuit(self):
        clock = FakeClock()
        gate = RequestGate(2, 60, clock=clock.now, sleep=clock.sleep,
                           jitter=lambda _start, _end: 0.0)
        self.assertTrue(gate.wait_for_slot())
        self.assertEqual(clock.value, 0.0)
        self.assertTrue(gate.wait_for_slot())
        self.assertEqual(clock.value, 30.0)
        self.assertFalse(gate.record_failure(blocked=True))
        self.assertFalse(gate.record_failure(blocked=True))
        self.assertTrue(gate.record_failure(blocked=True))
        self.assertFalse(gate.wait_for_slot())
        clock.sleep(3600)
        self.assertTrue(gate.wait_for_slot())

    def test_interleaved_blocks_also_open_circuit(self):
        clock = FakeClock()
        gate = RequestGate(6, 60, clock=clock.now, sleep=clock.sleep,
                           jitter=lambda _start, _end: 0.0)
        for _ in range(4):
            self.assertFalse(gate.record_failure(blocked=True))
            gate.record_success()
        self.assertTrue(gate.record_failure(blocked=True))
        self.assertFalse(gate.wait_for_slot())

    def test_missing_prices_among_successes_pause_degraded_source(self):
        clock = FakeClock()
        gate = RequestGate(2, 60, clock=clock.now, sleep=clock.sleep,
                           jitter=lambda _start, _end: 0.0)
        for _ in range(9):
            self.assertFalse(gate.record_failure(blocked=False))
            gate.record_success()
        self.assertTrue(gate.record_failure(blocked=False))
        self.assertFalse(gate.wait_for_slot())


if __name__ == "__main__":
    unittest.main()
