#!/usr/bin/env python3
"""Build or verify the standalone AHSANLY AI project guide."""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

SECTIONS = [
    "01_PRODUCT.md",
    "02_ARCHITECTURE.md",
    "03_FRONTEND.md",
    "04_API.md",
    "05_DATABASE.md",
    "06_DEAL_PIPELINE.md",
    "07_SECURITY_OPERATIONS.md",
    "08_DEVELOPMENT_TESTING.md",
    "09_DEPLOYMENT_ROLLBACK.md",
    "10_EXTENSION_GUIDE.md",
    "11_CURRENT_STATE_LIMITS.md",
    "12_AI_HANDOFF.md",
]

INTRODUCTION = """# AHSANLY Complete Project and AI Handoff Guide

This standalone document describes the current AHSANLY product, architecture, data model, editorial rules, public interfaces, operations, testing, deployment, and safe extension process. It is generated from the numbered files in `docs/` and can be shared with another AI without any additional project files.

It contains no credentials, customer data, database dumps, or evidence images. Dated observations are not permanent health claims: inspect the current source and reconfirm live state before making operational changes.
"""


def slugify(title: str) -> str:
    slug = re.sub(r"[^a-z0-9 -]", "", title.lower())
    return re.sub(r"[ -]+", "-", slug).strip("-")


def read_sections(docs_dir: Path) -> list[tuple[str, str, str]]:
    sections: list[tuple[str, str, str]] = []
    anchors: set[str] = set()
    for filename in SECTIONS:
        path = docs_dir / filename
        if not path.is_file():
            raise ValueError(f"Missing documentation section: {path}")
        text = path.read_text(encoding="utf-8").replace("\r\n", "\n").replace("\r", "\n").strip()
        if not text:
            raise ValueError(f"Empty documentation section: {path}")
        first_line = text.splitlines()[0]
        if not first_line.startswith("# "):
            raise ValueError(f"Section must start with one H1 title: {path}")
        title = first_line[2:].strip()
        anchor = slugify(title)
        if not anchor or anchor in anchors:
            raise ValueError(f"Missing or duplicate section anchor: {path}")
        anchors.add(anchor)
        sections.append((filename, title, text))
    return sections


def build(docs_dir: Path) -> str:
    sections = read_sections(docs_dir)
    toc = "\n".join(
        f"{index}. [{title}](#{slugify(title)})"
        for index, (_, title, _) in enumerate(sections, start=1)
    )
    parts = [INTRODUCTION.strip(), f"## Contents\n\n{toc}"]
    for filename, _, text in sections:
        parts.append(f"<!-- Source: docs/{filename} -->\n\n{text}")
    return "\n\n---\n\n".join(parts) + "\n"


def write_text(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8", newline="\n") as handle:
        handle.write(content)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="fail if the output differs from the documentation sources")
    parser.add_argument("--output", type=Path, help="write to this file instead of the repository root")
    args = parser.parse_args()

    docs_dir = Path(__file__).resolve().parent
    root = docs_dir.parent
    output = args.output if args.output is not None else root / "AHSANLY_AI_GUIDE.md"
    if not output.is_absolute():
        output = (Path.cwd() / output).resolve()

    try:
        expected = build(docs_dir)
        if args.check:
            if not output.is_file():
                raise ValueError(f"Generated guide does not exist: {output}")
            actual = output.read_text(encoding="utf-8").replace("\r\n", "\n").replace("\r", "\n")
            if actual != expected:
                raise ValueError(
                    f"Generated guide is stale: {output}. Run python docs/build_docs.py and commit the result."
                )
            print(f"Documentation is current: {output}")
            return 0
        write_text(output, expected)
        print(f"Generated {output} from {len(SECTIONS)} sections ({len(expected.encode('utf-8'))} bytes).")
        return 0
    except (OSError, UnicodeError, ValueError) as error:
        print(f"Documentation build failed: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
