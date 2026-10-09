"""Application root for source runs and the frozen release exe."""

from __future__ import annotations

import sys
from pathlib import Path


def app_root() -> Path:
    if getattr(sys, "frozen", False):
        return Path(sys.executable).resolve().parent
    return Path(__file__).resolve().parent


ROOT = app_root()
DATA = ROOT / "data"
CERTS = ROOT / "certs"
SAVES = ROOT / "saves"
RUNTIME = ROOT / "runtime"
ADMIN_UI = ROOT / "admin_ui"
