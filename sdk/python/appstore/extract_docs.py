"""Extract SDK method documentation for the IDE.

Uses AST to parse BaseApp (base.py), AppInputs (inputs.py), and AppLogger
(logging.py) to extract method signatures and docstrings.

Output: JSON array to stdout.  Each entry has:
  - name:        lookup key (e.g. "apt_install", "inputs.string", "log.info")
  - signature:   full signature with self prefix
  - description: first paragraph of docstring
  - group:       category for SDK reference panel display
"""

import ast
import json
import os
import textwrap


def _unparse(node):
    """Safely unparse an AST node to source text."""
    if node is None:
        return ""
    try:
        return ast.unparse(node)
    except Exception:
        return ""


def extract_signature(node, sig_prefix="self"):
    """Build a readable signature string from a FunctionDef."""
    args = node.args
    parts = []

    # Positional args (skip self/cls)
    positional = [a for a in args.args if a.arg not in ("self", "cls")]
    num_defaults = len(args.defaults)
    non_default_start = len(positional) - num_defaults

    for i, arg in enumerate(positional):
        s = arg.arg
        ann = _unparse(arg.annotation)
        if ann:
            s += f": {ann}"
        di = i - non_default_start
        if di >= 0:
            s += f" = {_unparse(args.defaults[di])}"
        parts.append(s)

    # *args
    if args.vararg:
        s = f"*{args.vararg.arg}"
        ann = _unparse(args.vararg.annotation)
        if ann:
            s += f": {ann}"
        parts.append(s)

    # keyword-only args
    for i, arg in enumerate(args.kwonlyargs):
        s = arg.arg
        ann = _unparse(arg.annotation)
        if ann:
            s += f": {ann}"
        default = args.kw_defaults[i]
        if default is not None:
            s += f" = {_unparse(default)}"
        parts.append(s)

    # **kwargs
    if args.kwarg:
        s = f"**{args.kwarg.arg}"
        ann = _unparse(args.kwarg.annotation)
        if ann:
            s += f": {ann}"
        parts.append(s)

    ret = ""
    ret_ann = _unparse(node.returns)
    if ret_ann:
        ret = f" -> {ret_ann}"

    param_str = ", ".join(parts)
    return f"{sig_prefix}.{node.name}({param_str}){ret}"


def extract_docstring(node):
    """Extract docstring text up to Args/Returns/Raises sections.

    Joins prose lines within paragraphs but preserves list items
    (lines starting with -) on separate lines.
    """
    if not node.body:
        return ""
    first = node.body[0]
    if not (isinstance(first, ast.Expr) and
            isinstance(first.value, ast.Constant) and
            isinstance(first.value.value, str)):
        return ""

    doc = textwrap.dedent(first.value.value).strip()

    # Stop at Args:/Returns:/Raises: sections
    lines = doc.splitlines()
    kept = []
    for line in lines:
        stripped = line.strip()
        if stripped.rstrip(":") in ("Args", "Returns", "Raises", "Yields", "Examples"):
            break
        kept.append(stripped)

    # Remove trailing empty lines
    while kept and not kept[-1]:
        kept.pop()
    if not kept:
        return ""

    # Group into paragraphs separated by blank lines
    paragraphs: list[list[str]] = []
    current: list[str] = []
    for line in kept:
        if not line:
            if current:
                paragraphs.append(current)
                current = []
        else:
            current.append(line)
    if current:
        paragraphs.append(current)

    # Join prose lines within each paragraph, but keep list items separate
    result = []
    for para in paragraphs:
        if any(l.startswith(("- ", "* ")) for l in para):
            result.append("\n".join(para))
        else:
            result.append(" ".join(para))

    return "\n\n".join(result)


def extract_class_methods(source, class_name, skip=None,
                          name_prefix="", sig_prefix="self",
                          groups=None, default_group="Other"):
    """Extract public method docs from a class."""
    skip = skip or set()
    groups = groups or {}
    tree = ast.parse(source)
    results = []
    for node in ast.walk(tree):
        if not isinstance(node, ast.ClassDef) or node.name != class_name:
            continue
        for item in node.body:
            if not isinstance(item, ast.FunctionDef):
                continue
            if item.name.startswith("_") or item.name in skip:
                continue
            full_name = f"{name_prefix}{item.name}"
            results.append({
                "name": full_name,
                "signature": extract_signature(item, sig_prefix),
                "description": extract_docstring(item),
                "group": groups.get(item.name, default_group),
            })
    return results


def main():
    sdk_dir = os.path.dirname(os.path.abspath(__file__))
    methods = []

    # Group mapping for BaseApp methods
    base_groups = {
        "apt_install": "Package Management",
        "pkg_install": "Package Management",
        "pip_install": "Package Management",
        "create_venv": "Package Management",
        "add_apt_key": "Package Management",
        "add_apt_repo": "Package Management",
        "add_apt_repository": "Package Management",
        "pull_oci_binary": "Package Management",
        "write_config": "File Operations",
        "render_template": "File Operations",
        "provision_file": "File Operations",
        "deploy_provision_file": "File Operations",
        "write_env_file": "File Operations",
        "create_dir": "File Operations",
        "chown": "File Operations",
        "download": "File Operations",
        "create_service": "Service Management",
        "enable_service": "Service Management",
        "restart_service": "Service Management",
        "run_command": "Commands & System",
        "run_shell": "Commands & System",
        "run_installer_script": "Commands & System",
        "sysctl": "Commands & System",
        "disable_ipv6": "Commands & System",
        "wait_for_http": "Commands & System",
        "create_user": "User Management",
        "status_page": "Advanced",
    }

    with open(os.path.join(sdk_dir, "base.py")) as f:
        methods.extend(extract_class_methods(
            f.read(), "BaseApp",
            skip={"install", "configure", "healthcheck", "uninstall"},
            groups=base_groups,
        ))

    with open(os.path.join(sdk_dir, "inputs.py")) as f:
        methods.extend(extract_class_methods(
            f.read(), "AppInputs",
            skip={"from_file"},
            name_prefix="inputs.", sig_prefix="self.inputs",
            default_group="Inputs",
        ))

    with open(os.path.join(sdk_dir, "logging.py")) as f:
        methods.extend(extract_class_methods(
            f.read(), "AppLogger",
            name_prefix="log.", sig_prefix="self.log",
            default_group="Logging & Outputs",
        ))

    print(json.dumps(methods))


if __name__ == "__main__":
    main()
