"""Package only new platform source files; never include local databases or secrets."""
from pathlib import Path
import tarfile

ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / "data" / "deal-system-source.tar.gz"
OUTPUT.parent.mkdir(exist_ok=True)
with tarfile.open(OUTPUT, "w:gz") as archive:
    for path in sorted((ROOT / "deal_system").rglob("*")):
        if path.is_file() and path.suffix in (".py", ".html"):
            archive.add(path, arcname=path.relative_to(ROOT).as_posix())
    for name in ("manage.py", "deal_requirements.txt"):
        archive.add(ROOT / name, arcname=name)
print(OUTPUT)
