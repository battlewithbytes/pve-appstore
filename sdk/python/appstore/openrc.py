"""OpenRC init script generator for create_service() on Alpine."""


def generate_init_script(
    name: str,
    exec_start: str,
    description: str = None,
    after: str = None,
    user: str = None,
    working_directory: str = None,
    environment: dict = None,
    environment_file: str = None,
    restart: str = "always",
    capabilities: list = None,
    extra_service: str = None,
) -> str:
    """Generate an OpenRC init script string.

    Args:
        name: Service name (used for process name).
        exec_start: Command to run.
        description: Service description.
        after: OpenRC dependency (maps to 'need' directive).
        user: User to run as (None = root).
        working_directory: Directory to chdir into.
        environment: Dict of KEY=VALUE env vars.
        environment_file: Path to env file to source.
        restart: Restart policy ('always' enables respawn).
        capabilities: Ignored on OpenRC (no direct equivalent).
        extra_service: Extra lines for start() function body.

    Returns:
        Complete OpenRC init script as a string.
    """
    desc = description or name

    lines = ["#!/sbin/openrc-run", ""]
    lines.append(f'description="{desc}"')

    # Split exec_start into command + command_args for OpenRC
    parts = exec_start.split(None, 1)
    lines.append(f"command={parts[0]}")
    if len(parts) > 1:
        lines.append(f'command_args="{parts[1]}"')
    lines.append("command_background=true")
    lines.append(f"pidfile=/run/{name}.pid")

    if user:
        lines.append(f"command_user={user}")

    # Route stdout/stderr to syslog so `tail -f /var/log/messages` shows app logs
    lines.append(f'output_logger="logger -t {name} -p daemon.info"')
    lines.append(f'error_logger="logger -t {name} -p daemon.err"')

    # Dependencies
    deps = ["net"]
    if after and after != "network-online.target":
        # Convert systemd target to openrc service name
        dep = after.replace(".target", "").replace(".service", "")
        deps.append(dep)
    lines.append("")
    lines.append("depend() {")
    lines.append(f"    need {' '.join(deps)}")
    lines.append("}")

    # Start function with env setup
    lines.append("")
    lines.append("start_pre() {")
    if environment_file:
        lines.append(f'    [ -f "{environment_file}" ] && . "{environment_file}"')
    if environment:
        for k, v in environment.items():
            lines.append(f'    export {k}="{v}"')
    if working_directory:
        lines.append(f'    cd "{working_directory}"')
    if extra_service:
        lines.append(f"    {extra_service}")
    lines.append("    return 0")
    lines.append("}")
    lines.append("")

    # Supervisor for restart
    if restart == "always":
        lines.append("supervisor=supervise-daemon")
        lines.append("")

    return "\n".join(lines)
