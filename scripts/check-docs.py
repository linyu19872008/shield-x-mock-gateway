#!/usr/bin/env python3
"""Validate relative Markdown links, language switches, and bilingual mirrors."""
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parent.parent
errors: list[str] = []

PAIRS = [
    "README.md",
    "docs/README.md",
    "docs/quickstart.md",
    "docs/cli.md",
    "docs/configuration.md",
    "docs/data-plane.md",
    "docs/troubleshooting.md",
    "docs/architecture.md",
    "docs/design.md",
    "docs/contract/agent-v1.md",
    "testdata/template-conformance/README.md",
]

LINK_RE = re.compile(r"\[[^\]]*\]\(([^)]+)\)")

def check_links(path: Path) -> None:
    text = path.read_text(encoding="utf-8", errors="replace")
    for match in LINK_RE.finditer(text):
        target = match.group(1).split("#", 1)[0].strip()
        if not target or target.startswith(("http://", "https://", "mailto:", "#")):
            continue
        if target.startswith("/"):
            target = target.lstrip("/")
        resolved = (path.parent / target).resolve()
        if not resolved.exists():
            errors.append(f"{path.relative_to(ROOT)}: broken link {target}")

for path in ROOT.rglob("*.md"):
    if ".git" in path.parts:
        continue
    check_links(path)

for english in PAIRS:
    en = ROOT / english
    zh = en.with_name(en.stem + ".zh-CN" + en.suffix)
    if not en.exists():
        errors.append(f"missing English document: {english}")
        continue
    if not zh.exists():
        errors.append(f"missing Chinese mirror: {zh.relative_to(ROOT)}")
        continue
    en_text = en.read_text(encoding="utf-8")
    zh_text = zh.read_text(encoding="utf-8")
    if zh.name not in en_text:
        errors.append(f"{english}: missing link to {zh.name}")
    if en.name not in zh_text:
        errors.append(f"{zh.relative_to(ROOT)}: missing link to {en.name}")

for path in ROOT.rglob("*.md"):
    if ".git" in path.parts:
        continue
    text = path.read_text(encoding="utf-8", errors="replace")
    for forbidden in ("make mock-build", "https://www.baidu.com", "tools/mock-gateway"):
        if forbidden in text:
            errors.append(f"{path.relative_to(ROOT)}: stale reference {forbidden!r}")
    if re.search(r"sx_[A-Za-z0-9_-]{20,}", text):
        errors.append(f"{path.relative_to(ROOT)}: possible token in documentation")

if errors:
    print("docs-check failed:")
    for err in errors:
        print(" -", err)
    sys.exit(1)
print(f"docs-check passed: {len(PAIRS)} English/Chinese pairs, all relative links resolve")
