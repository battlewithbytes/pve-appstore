"""Input handling for app provisioning scripts.

Reads a JSON file containing user-provided inputs with typed accessors.
"""

import json


class AppInputs:
    """Typed accessor for app inputs loaded from a JSON file."""

    def __init__(self, data: dict):
        self._data = data

    @classmethod
    def from_file(cls, path: str) -> "AppInputs":
        with open(path, "r") as f:
            return cls(json.load(f))

    def string(self, key: str, default: str = "") -> str:
        """Get a string input value. Returns default if the key was not provided."""
        val = self._data.get(key)
        if val is None:
            return default
        return str(val)

    def integer(self, key: str, default: int = 0) -> int:
        """Get an integer input value. Returns default if the key was not provided."""
        val = self._data.get(key)
        if val is None:
            return default
        return int(val)

    def boolean(self, key: str, default: bool = False) -> bool:
        """Get a boolean input value. Accepts true/1/yes as truthy strings."""
        val = self._data.get(key)
        if val is None:
            return default
        if isinstance(val, bool):
            return val
        if isinstance(val, str):
            return val.lower() in ("true", "1", "yes")
        return bool(val)

    def secret(self, key: str, default: str = "") -> str:
        """Same as string but indicates the value should be redacted in logs."""
        return self.string(key, default)

    def raw(self) -> dict:
        """Return the raw input dictionary."""
        return dict(self._data)
