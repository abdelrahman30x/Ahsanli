"""Server-only PostgreSQL role and secret provisioning without printing secret values."""
import os
import secrets
import subprocess
from pathlib import Path

BASE = Path("/root/botAfflite")
ENV_FILE = BASE / "deal.env"
ADMIN_FILE = BASE / "data" / "deal-admin-password"


def psql(sql):
    subprocess.run(["runuser", "-u", "postgres", "--", "psql", "-v", "ON_ERROR_STOP=1", "-q"],
                   input=sql, text=True, check=True, stdout=subprocess.DEVNULL)


if not ENV_FILE.exists():
    db_password = secrets.token_urlsafe(36)
    django_secret = secrets.token_urlsafe(64)
    role_exists = subprocess.run(["runuser", "-u", "postgres", "--", "psql", "-tAc",
                                  "SELECT 1 FROM pg_roles WHERE rolname='botafflite'"],
                                 check=True, capture_output=True, text=True).stdout.strip() == "1"
    psql(f"{'ALTER ROLE' if role_exists else 'CREATE ROLE'} botafflite LOGIN PASSWORD '{db_password}';")
    database_exists = subprocess.run(["runuser", "-u", "postgres", "--", "psql", "-tAc",
                                      "SELECT 1 FROM pg_database WHERE datname='botafflite'"],
                                     check=True, capture_output=True, text=True).stdout.strip() == "1"
    if not database_exists:
        subprocess.run(["runuser", "-u", "postgres", "--", "createdb", "-O", "botafflite", "botafflite"], check=True)
    ENV_FILE.write_text("\n".join([
        f"DEAL_SECRET_KEY={django_secret}", f"DEAL_DB_PASSWORD={db_password}",
        "DEAL_DB_NAME=botafflite", "DEAL_DB_USER=botafflite", "DEAL_DB_HOST=127.0.0.1",
        "DEAL_ALLOWED_HOSTS=134.122.89.95,127.0.0.1", "DEAL_CSRF_ORIGINS=https://134.122.89.95",
        "DEAL_PUBLISH_ENABLED=0", "DEAL_ADMIN_PASSWORD_FILE=/root/botAfflite/data/deal-admin-password", ""]), encoding="utf-8")
    ENV_FILE.chmod(0o600)
if not ADMIN_FILE.exists():
    ADMIN_FILE.parent.mkdir(parents=True, exist_ok=True)
    ADMIN_FILE.write_text(secrets.token_urlsafe(24) + "\n", encoding="utf-8")
    ADMIN_FILE.chmod(0o600)
print("Deal database and root-owned environment/password files are ready; no secret values were displayed")
