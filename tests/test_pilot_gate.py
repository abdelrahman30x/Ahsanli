import unittest
from datetime import datetime, timedelta, timezone

from hashlib import sha256

from deal_system.core.pilot_gate import evaluate_pilot, rotated_bot_token


class PilotGateTests(unittest.TestCase):
    def setUp(self):
        self.now = datetime(2026, 11, 4, 2, tzinfo=timezone.utc)

    def report(self, hours_ago, valid=19):
        return {
            "checked_at": (self.now - timedelta(hours=hours_ago)).isoformat(),
            "sample_count": 20,
            "results": [
                {"listing_id": number, "result": "valid" if number < valid else "failure"}
                for number in range(20)
            ],
        }

    def test_recent_complete_pilot_ignores_older_failures_and_spot_checks(self):
        reports = [self.report(hours) for hours in (61, 49, 37, 25, 13, 1)]
        reports.append(self.report(80, valid=0))
        reports.append({"checked_at": (self.now - timedelta(hours=5)).isoformat(),
                        "sample_count": 1, "results": [{"listing_id": 1, "result": "failure"}]})
        self.assertEqual(evaluate_pilot(reports, self.now), (20, 0.95, True))

    def test_high_rate_without_48_hour_span_is_not_ready(self):
        reports = [self.report(hours, valid=20) for hours in (37, 25, 13, 1)]
        self.assertEqual(evaluate_pilot(reports, self.now), (20, 1.0, False))

    def test_stale_pilot_is_not_ready(self):
        reports = [self.report(hours, valid=20) for hours in (70, 58, 46, 34)]
        self.assertEqual(evaluate_pilot(reports, self.now), (20, 1.0, False))

    def test_token_rotation_requires_new_matching_go_and_legacy_values(self):
        old = "1234567890:" + "A" * 35
        new = "1234567890:" + "B" * 35
        fingerprint = sha256(old.encode()).hexdigest()
        self.assertFalse(rotated_bot_token(fingerprint, old, old))
        self.assertFalse(rotated_bot_token(fingerprint, new, old))
        self.assertTrue(rotated_bot_token(fingerprint, new, new))


if __name__ == "__main__":
    unittest.main()
