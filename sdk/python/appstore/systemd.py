"""Systemd unit file generator for create_service()."""


def generate_service_unit(
    name: str,
    exec_start: str,
    description: str = None,
    after: str = "network-online.target",
    user: str = None,
    working_directory: str = None,
    environment: dict = None,
    environment_file: str = None,
    restart: str = "always",
    restart_sec: int = 5,
    type: str = "simple",
    capabilities: list = None,
    extra_unit: str = None,
    extra_service: str = None,
) -> str:
    """Generate a systemd service unit file string.

    Args:
        name: Service name (used only for default Description).
        exec_start: Command to run.
        description: Unit description.
        after: After= dependency.
        user: User= directive (None = root).
        working_directory: WorkingDirectory= directive.
        environment: Dict of KEY=VALUE pairs for Environment= lines.
        environment_file: Path to EnvironmentFile=.
        restart: Restart= policy.
        restart_sec: RestartSec= in seconds.
        type: Service Type=.
        capabilities: List of capabilities (e.g. ["CAP_NET_ADMIN"]).
        extra_unit: Raw lines appended to [Unit].
        extra_service: Raw lines appended to [Service].

    Returns:
        Complete systemd unit file as a string.
    """
    desc = description or name

    # [Unit]
    lines = ["[Unit]", f"Description={desc}"]
    if after:
        lines.append(f"After={after}")
        lines.append(f"Wants={after}")
    if extra_unit:
        lines.append(extra_unit)

    # [Service]
    lines.append("")
    lines.append("[Service]")
    lines.append(f"Type={type}")
    lines.append(f"ExecStart={exec_start}")

    if user:
        lines.append(f"User={user}")
    if working_directory:
        lines.append(f"WorkingDirectory={working_directory}")
    if environment:
        for k, v in environment.items():
            lines.append(f'Environment="{k}={v}"')
    if environment_file:
        lines.append(f"EnvironmentFile={environment_file}")

    lines.append(f"Restart={restart}")
    lines.append(f"RestartSec={restart_sec}")

    if capabilities:
        ambient = " ".join(capabilities)
        lines.append(f"AmbientCapabilities={ambient}")
        lines.append(f"CapabilityBoundingSet={ambient}")

    if extra_service:
        lines.append(extra_service)

    # [Install]
    lines.append("")
    lines.append("[Install]")
    lines.append("WantedBy=multi-user.target")
    lines.append("")

    return "\n".join(lines)
