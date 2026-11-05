"""Exercise the live HTTPS login and OTP flow without printing credentials."""
import os
import sys
import time
from pathlib import Path
import requests
from bs4 import BeautifulSoup

base = "https://134.122.89.95"
password = Path(os.environ["DEAL_ADMIN_PASSWORD_FILE"]).read_text(encoding="utf-8").strip()
with requests.Session() as session:
    login_page = session.get(base + "/login/", timeout=15)
    login_page.raise_for_status()
    soup = BeautifulSoup(login_page.text, "html.parser")
    csrf = soup.select_one('input[name="csrfmiddlewaretoken"]')["value"]
    response = session.post(base + "/login/", data={
        "username": "dealadmin", "password": password, "csrfmiddlewaretoken": csrf,
    }, headers={"Referer": base + "/login/"}, timeout=15)
    if response.status_code != 200 or "/otp/" not in response.url:
        raise RuntimeError(f"Dashboard login did not reach OTP setup: HTTP {response.status_code}")
    if "data:image/png;base64," in response.text:
        print("HTTPS password login and TOTP setup page: OK")
    else:
        # Once a device is confirmed the QR must disappear. Verify that its
        # current code can complete login and reach the actual dashboard.
        otp_csrf = BeautifulSoup(response.text, "html.parser").select_one(
            'input[name="csrfmiddlewaretoken"]'
        )
        if not otp_csrf or 'name="token"' not in response.text:
            raise RuntimeError("OTP challenge did not render")
        sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
        os.environ.setdefault("DJANGO_SETTINGS_MODULE", "deal_system.settings")
        import django
        django.setup()
        from django_otp.oath import TOTP
        from django_otp.plugins.otp_totp.models import TOTPDevice

        device = TOTPDevice.objects.get(user__username="dealadmin", confirmed=True)
        authenticator = TOTP(device.bin_key, device.step, device.t0, device.digits, device.drift)
        if authenticator.t() <= device.last_t:
            wait_seconds = (device.last_t + 1 - device.drift) * device.step + device.t0 - time.time() + 1
            time.sleep(max(0, min(wait_seconds, device.step + 1)))
        response = session.post(base + "/otp/", data={
            "token": str(authenticator.token()).zfill(device.digits),
            "csrfmiddlewaretoken": otp_csrf["value"],
        }, headers={"Referer": base + "/otp/"}, timeout=15)
        if (response.status_code != 200 or "<title>BotAfflite deals</title>" not in response.text
                or "Kanbkam discovery" not in response.text):
            raise RuntimeError(f"OTP login did not reach dashboard: HTTP {response.status_code}")
        print("HTTPS password login, OTP verification, and dashboard: OK")
