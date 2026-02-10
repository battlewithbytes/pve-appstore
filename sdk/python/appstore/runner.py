"""Entry point for running app provisioning scripts.

The Go engine invokes this via:
    python3 -m appstore.runner <inputs.json> <permissions.json> <action> <app_module>

The app_module is the Python file path (e.g., /opt/appstore/provision/install.py)
which must define an app class registered via appstore.run().
"""

import importlib.util
import sys
import traceback

from appstore.inputs import AppInputs
from appstore.logging import AppLogger
from appstore.permissions import AppPermissions, PermissionDeniedError

# Global reference set by run() in the app script
_app_class = None


def run(app_class):
    """Register an app class and execute the requested lifecycle action.

    Called at the bottom of each app's install.py:
        from appstore import BaseApp, run
        class MyApp(BaseApp): ...
        run(MyApp)
    """
    global _app_class
    _app_class = app_class


def main():
    log = AppLogger()

    if len(sys.argv) < 5:
        log.error(
            f"Usage: python3 -m appstore.runner <inputs.json> <permissions.json> <action> <app_module>"
        )
        sys.exit(1)

    inputs_path = sys.argv[1]
    permissions_path = sys.argv[2]
    action = sys.argv[3]
    app_module_path = sys.argv[4]

    valid_actions = ("install", "configure", "healthcheck", "uninstall")
    if action not in valid_actions:
        log.error(f"Invalid action '{action}'. Must be one of: {valid_actions}")
        sys.exit(1)

    try:
        inputs = AppInputs.from_file(inputs_path)
        permissions = AppPermissions.from_file(permissions_path)
    except Exception as e:
        log.error(f"Failed to load inputs/permissions: {e}")
        sys.exit(1)

    # Load the app module dynamically
    try:
        spec = importlib.util.spec_from_file_location("app_module", app_module_path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
    except Exception as e:
        log.error(f"Failed to load app module {app_module_path}: {e}")
        sys.exit(1)

    # When run as `python3 -m appstore.runner`, the module may exist twice
    # in sys.modules (as 'appstore.runner' and '__main__'). The app's
    # `from appstore import run` calls run() on the 'appstore.runner' copy,
    # so check there if our local _app_class is still None.
    app_class = _app_class
    if app_class is None:
        runner_mod = sys.modules.get("appstore.runner")
        if runner_mod is not None:
            app_class = getattr(runner_mod, "_app_class", None)

    if app_class is None:
        log.error(
            f"App module {app_module_path} did not call appstore.run(AppClass)"
        )
        sys.exit(1)

    # Instantiate and run
    try:
        app = app_class(inputs, permissions)
        method = getattr(app, action, None)
        if method is None:
            log.error(f"App class does not implement '{action}' method")
            sys.exit(1)

        log.info(f"Starting {action}...")
        result = method()

        if action == "healthcheck":
            if result is False:
                log.warn("Healthcheck returned False")
                sys.exit(1)

        log.info(f"{action.capitalize()} completed successfully")
    except PermissionDeniedError as e:
        log.error(f"Permission denied: {e}")
        sys.exit(2)
    except Exception as e:
        log.error(f"{action} failed: {e}")
        traceback.print_exc(file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
