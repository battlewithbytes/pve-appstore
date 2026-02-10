"""Structured logging for app provisioning scripts.

All log output uses the @@APPLOG@@ prefix so the Go engine can parse
structured entries from the script's stdout.
"""

import json
import sys


class AppLogger:
    """Logger that emits structured JSON lines for the engine to parse."""

    def _emit(self, data: dict) -> None:
        line = json.dumps(data, separators=(",", ":"))
        print(f"@@APPLOG@@{line}", flush=True)

    def info(self, msg: str) -> None:
        self._emit({"level": "info", "msg": msg})

    def warn(self, msg: str) -> None:
        self._emit({"level": "warn", "msg": msg})

    def error(self, msg: str) -> None:
        self._emit({"level": "error", "msg": msg})

    def progress(self, step: int, total: int, msg: str) -> None:
        self._emit({"level": "info", "msg": msg, "progress": {"step": step, "total": total}})

    def output(self, key: str, value: str) -> None:
        """Emit a key-value output that the engine captures as a job output."""
        self._emit({"level": "output", "key": key, "value": value})
