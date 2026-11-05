"""Record a 48-hour kernel/systemd OOM audit for the release gate."""
import json
import re
import subprocess
from datetime import datetime, timedelta, timezone
from pathlib import Path

END = datetime.now(timezone.utc)
START = END - timedelta(hours=48)
since = START.strftime("%Y-%m-%d %H:%M:%S UTC")
until = END.strftime("%Y-%m-%d %H:%M:%S UTC")
text = subprocess.run(["journalctl", "--no-pager", "-k", "--since", since, "--until", until, "-o", "cat"],
                      check=True, capture_output=True, text=True).stdout
matches = re.findall(r"(?im)^.*(?:out of memory|oom-kill|killed process).*$", text)
service_since = {}
active = {}
for service in ("botafflite-go-api.service", "botafflite-next-preview.service",
                "botafflite-go-worker.service", "botafflite-go-pipeline.service",
                "botafflite-web.service",
                "amazon-tracker.service"):
    active[service] = subprocess.run(["systemctl", "is-active", "--quiet", service]).returncode == 0
    output = subprocess.run(["systemctl", "show", service, "--property=ActiveEnterTimestamp", "--value"],
                            check=True, capture_output=True, text=True).stdout.strip()
    try:
        service_since[service] = datetime.strptime(output, "%a %Y-%m-%d %H:%M:%S UTC").replace(tzinfo=timezone.utc).isoformat()
    except ValueError:
        service_since[service] = None
report = {"from": START.isoformat(), "to": END.isoformat(), "oom_count": len(matches),
          "source": "journalctl -k", "audited_at": END.isoformat(), "service_since": service_since,
          "active": active,
          "full_coverage": all(active.values()) and all(
              value and datetime.fromisoformat(value) <= START for value in service_since.values())}
target = Path("/root/botAfflite/data/pilot/oom_audit.json")
target.write_text(json.dumps(report, indent=2), encoding="utf-8")
target.chmod(0o600)
print(f"48-hour OOM audit: {len(matches)} kernel events; full service coverage={report['full_coverage']}; report {target}")
