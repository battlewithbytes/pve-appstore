"""Alpine Linux platform implementations for BaseApp.

Each function takes `app` (a BaseApp instance) as its first argument
and uses app.log, app._run, app.permissions, etc.
"""

import os
import subprocess

from appstore.openrc import generate_init_script


def pkg_install(app, packages):
    """Install packages via apk."""
    app.log.info(f"Installing packages (apk): {', '.join(packages)}")
    app._run(["apk", "add", "--no-cache"] + packages)


def enable_service(app, name):
    """Enable and start an OpenRC service."""
    app._run(["rc-update", "add", name, "default"])
    app._run(["rc-service", name, "start"])


def restart_service(app, name):
    """Restart an OpenRC service."""
    app._run(["rc-service", name, "restart"])


def create_service(app, name, exec_start, description=None,
                   after="network-online.target", user=None,
                   working_directory=None, environment=None,
                   environment_file=None, restart="always",
                   restart_sec=5, type="simple", capabilities=None,
                   extra_unit=None, extra_service=None):
    """Create, enable, and start an OpenRC service."""
    script = generate_init_script(
        name=name, exec_start=exec_start, description=description,
        after=after, user=user, working_directory=working_directory,
        environment=environment, environment_file=environment_file,
        restart=restart, capabilities=capabilities,
        extra_service=extra_service,
    )
    script_path = f"/etc/init.d/{name}"
    app.permissions.check_path(script_path)
    os.makedirs("/etc/init.d", exist_ok=True)
    with open(script_path, "w") as f:
        f.write(script)
    os.chmod(script_path, 0o755)
    app.log.info(f"Created OpenRC service: {name}")
    app._run(["rc-update", "add", name, "default"])
    app._run(["rc-service", name, "start"])


def create_user(app, name, system=True, home=None, shell="/bin/false"):
    """Create a user via adduser (Alpine)."""
    nologin = "/sbin/nologin" if shell == "/bin/false" else shell
    cmd = ["adduser", "-D"]
    if system:
        cmd.append("-S")
    if home:
        cmd.extend(["-h", home])
    else:
        cmd.extend(["-h", "/dev/null", "-H"])
    cmd.extend(["-s", nologin, name])
    result = subprocess.run(cmd, capture_output=True)
    if result.returncode != 0 and b"already exists" not in result.stderr and b"in use" not in result.stderr:
        raise RuntimeError(
            f"adduser failed: {result.stderr.decode().strip()}"
        )


def enable_repo(app, repo_name):
    """Enable a named repository by uncommenting it in /etc/apk/repositories."""
    repo_file = "/etc/apk/repositories"
    app.log.info(f"Enabling Alpine repository: {repo_name}")
    with open(repo_file) as f:
        lines = f.readlines()
    changed = False
    new_lines = []
    for line in lines:
        stripped = line.lstrip("#").strip()
        if repo_name in stripped and line.lstrip().startswith("#"):
            new_lines.append(stripped + "\n")
            changed = True
        else:
            new_lines.append(line)
    if changed:
        with open(repo_file, "w") as f:
            f.writelines(new_lines)
        app.log.info(f"Uncommented {repo_name} in {repo_file}")
    else:
        app.log.info(f"Repository {repo_name} already enabled or not found in {repo_file}")
