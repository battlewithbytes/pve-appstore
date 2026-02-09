"""PVE App Store SDK — Python provisioning framework for LXC apps."""

from appstore.base import BaseApp
from appstore.permissions import AppPermissions, PermissionDeniedError
from appstore.inputs import AppInputs

__all__ = ["BaseApp", "AppPermissions", "AppInputs", "PermissionDeniedError", "run"]


def __getattr__(name):
    if name == "run":
        from appstore.runner import run
        return run
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")
