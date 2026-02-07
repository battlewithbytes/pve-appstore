"""Simple config file templating using string.Template.

Uses $variable syntax (Python's built-in string.Template). This avoids
pulling in Jinja2 as a dependency while still supporting variable substitution.
"""

from string import Template


def render(template_str: str, **kwargs) -> str:
    """Render a template string with the given variables.

    Uses safe_substitute so missing variables are left as-is rather than
    raising an error.
    """
    return Template(template_str).safe_substitute(**kwargs)
