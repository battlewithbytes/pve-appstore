"""Simple config file templating.

Supports:
  - $variable or ${variable} substitution (Python string.Template)
  - {{#key}} ... {{/key}} conditional blocks (included when key is truthy)
  - {{^key}} ... {{/key}} inverted blocks (included when key is falsy)

This avoids pulling in Jinja2 while covering 95% of config file needs.
"""

import re
from string import Template


# Match {{#key}}...{{/key}} and {{^key}}...{{/key}} blocks (non-greedy, DOTALL)
_BLOCK_RE = re.compile(
    r"\{\{([#^])(\w+)\}\}\n?(.*?)\{\{/\2\}\}\n?",
    re.DOTALL,
)


def render(template_str: str, **kwargs) -> str:
    """Render a template string with variables and conditional blocks.

    1. Process {{#key}}...{{/key}} blocks (include if kwargs[key] is truthy)
    2. Process {{^key}}...{{/key}} blocks (include if kwargs[key] is falsy)
    3. Substitute $variable / ${variable} references
    """
    def _replace_block(m):
        tag = m.group(1)    # '#' or '^'
        key = m.group(2)
        body = m.group(3)
        val = kwargs.get(key)
        if tag == '#':
            return body if val else ""
        else:  # '^'
            return body if not val else ""

    result = _BLOCK_RE.sub(_replace_block, template_str)
    return Template(result).safe_substitute(**kwargs)
