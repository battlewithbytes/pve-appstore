"""OS detection for portable install scripts.

Reads /etc/os-release to determine whether the container runs Debian or Alpine.
"""


def detect_os() -> str:
    """Read /etc/os-release and return 'debian', 'alpine', or 'unknown'."""
    try:
        with open("/etc/os-release") as f:
            content = f.read().lower()
    except FileNotFoundError:
        return "unknown"

    # Check ID= line first for precise match
    for line in content.splitlines():
        if line.startswith("id="):
            val = line.split("=", 1)[1].strip().strip('"')
            if val in ("debian", "ubuntu"):
                return "debian"
            if val == "alpine":
                return "alpine"

    # Fallback: check ID_LIKE= line
    for line in content.splitlines():
        if line.startswith("id_like="):
            val = line.split("=", 1)[1].strip().strip('"')
            if "debian" in val:
                return "debian"
            if "alpine" in val:
                return "alpine"

    return "unknown"
