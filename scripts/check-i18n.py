#!/usr/bin/env python3
"""Check the vanilla UI catalogs and static translation keys."""
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parent.parent
WEB = ROOT / "internal/control/web"
errors: list[str] = []

def keys(path: Path) -> set[str]:
    text = path.read_text(encoding="utf-8")
    return set(re.findall(r'^\s*"([^"]+)":', text, re.M))

en = keys(WEB / "locales/en.js")
zh = keys(WEB / "locales/zh-CN.js")
if en != zh:
    errors.append(f"catalog key mismatch: missing in zh={sorted(en - zh)} missing in en={sorted(zh - en)}")

used: set[str] = set()
for name in ("app.js", "index.html", "i18n.js"):
    text = (WEB / name).read_text(encoding="utf-8")
    used |= set(re.findall(r'\btr\("([^"]+)"', text))
    used |= set(re.findall(r'data-i18n(?:-aria-label|-placeholder)?="([^"]+)"', text))

# Dynamic keys are assembled from known prefixes.
dynamic_prefixes = ("theme.", "api.error.")
static_used = {k for k in used if not k.startswith(dynamic_prefixes)}
missing = sorted(static_used - en)
if missing:
    errors.append(f"keys used by UI but missing from catalogs: {missing}")

for prefix in ("theme.", "api.error."):
    if not any(k.startswith(prefix) for k in en):
        errors.append(f"no catalog entries for {prefix}")

for path in (WEB / "locales/en.js", WEB / "locales/zh-CN.js"):
    for i, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        if re.match(r'\s*"[^"]+":\s*""', line):
            errors.append(f"{path.relative_to(ROOT)}:{i}: empty translation")

if errors:
    print("i18n-check failed:")
    for err in errors:
        print(" -", err)
    sys.exit(1)
print(f"i18n-check passed: {len(en)} keys in both catalogs, {len(static_used)} static keys used")
