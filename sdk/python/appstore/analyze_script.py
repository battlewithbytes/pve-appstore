"""AST-based script analyzer for install.py validation.

Extracts structured facts from a Python install script:
- imports from the appstore module
- class name (BaseApp subclass)
- structural booleans (has install method, has run() call)
- input keys referenced via self.inputs.<type>("key")
- all self.<method>(...) calls with resolved args/kwargs
- unsafe patterns (os.system, subprocess)

Output: JSON to stdout. On syntax error, returns {"error": "..."}.
Usage: python3 analyze_script.py <script_path>
"""

import ast
import json
import sys


def _resolve_value(node):
    """Resolve an AST node to a Python literal, or '<dynamic>' if not static."""
    if isinstance(node, ast.Constant):
        return node.value
    if isinstance(node, ast.List):
        return [_resolve_value(el) for el in node.elts]
    if isinstance(node, ast.Tuple):
        return [_resolve_value(el) for el in node.elts]
    if isinstance(node, ast.Dict):
        result = {}
        for k, v in zip(node.keys, node.values):
            key = _resolve_value(k)
            if isinstance(key, str):
                result[key] = _resolve_value(v)
        return result
    if isinstance(node, ast.UnaryOp) and isinstance(node.op, ast.USub):
        val = _resolve_value(node.operand)
        if isinstance(val, (int, float)):
            return -val
    return "<dynamic>"


class ScriptAnalyzer(ast.NodeVisitor):
    def __init__(self):
        self.imports = []
        self.class_name = ""
        self.has_install_method = False
        self.has_run_call = False
        self.input_keys = []
        self.method_calls = []
        self.unsafe_patterns = []
        self.defined_methods = []
        self._in_class = False

    def visit_ImportFrom(self, node):
        if node.module == "appstore":
            for alias in node.names:
                self.imports.append(alias.name)
        self.generic_visit(node)

    def visit_ClassDef(self, node):
        for base in node.bases:
            name = ""
            if isinstance(base, ast.Name):
                name = base.id
            elif isinstance(base, ast.Attribute):
                name = base.attr
            if name == "BaseApp":
                self.class_name = node.name
                self._in_class = True
                for item in node.body:
                    if isinstance(item, ast.FunctionDef):
                        if item.name == "install":
                            self.has_install_method = True
                        self.defined_methods.append(item.name)
                self.generic_visit(node)
                self._in_class = False
                return
        self.generic_visit(node)

    def visit_Call(self, node):
        # Check for run(ClassName) at module level
        if isinstance(node.func, ast.Name) and node.func.id == "run":
            if node.args:
                arg = node.args[0]
                if isinstance(arg, ast.Name) and arg.id == self.class_name:
                    self.has_run_call = True

        # Check for self.<method>(...) calls
        if isinstance(node.func, ast.Attribute):
            value = node.func.value

            # self.inputs.<type>("key", ...)
            if (isinstance(value, ast.Attribute) and
                isinstance(value.value, ast.Name) and
                value.value.id == "self" and
                value.attr == "inputs"):
                input_type = node.func.attr
                if node.args:
                    key = _resolve_value(node.args[0])
                    if isinstance(key, str) and key != "<dynamic>":
                        self.input_keys.append({
                            "key": key,
                            "line": node.lineno,
                            "type": input_type,
                        })

            # self.<method>(...) — record all method calls on self
            if isinstance(value, ast.Name) and value.id == "self":
                method = node.func.attr
                args = []
                for a in node.args:
                    if isinstance(a, ast.Starred):
                        args.append("<dynamic>")
                    else:
                        args.append(_resolve_value(a))
                kwargs = {}
                for kw in node.keywords:
                    if kw.arg is not None:
                        kwargs[kw.arg] = _resolve_value(kw.value)
                self.method_calls.append({
                    "method": method,
                    "line": node.lineno,
                    "args": args,
                    "kwargs": kwargs,
                })

            # Unsafe: os.system(...)
            if (isinstance(value, ast.Name) and value.id == "os" and
                    node.func.attr == "system"):
                self.unsafe_patterns.append({
                    "line": node.lineno,
                    "pattern": "os.system",
                })

            # Unsafe: subprocess.call/run(...)
            if (isinstance(value, ast.Name) and value.id == "subprocess" and
                    node.func.attr in ("call", "run", "Popen", "check_call", "check_output")):
                self.unsafe_patterns.append({
                    "line": node.lineno,
                    "pattern": f"subprocess.{node.func.attr}",
                })

        self.generic_visit(node)


def analyze(source, filename="<script>"):
    result = {
        "imports": [],
        "class_name": "",
        "has_install_method": False,
        "has_run_call": False,
        "input_keys": [],
        "method_calls": [],
        "unsafe_patterns": [],
        "defined_methods": [],
        "error": None,
    }
    try:
        tree = ast.parse(source, filename=filename)
    except SyntaxError as e:
        result["error"] = f"SyntaxError: {e.msg} (line {e.lineno})"
        return result

    analyzer = ScriptAnalyzer()
    analyzer.visit(tree)

    result["imports"] = analyzer.imports
    result["class_name"] = analyzer.class_name
    result["has_install_method"] = analyzer.has_install_method
    result["has_run_call"] = analyzer.has_run_call
    result["input_keys"] = analyzer.input_keys
    result["method_calls"] = analyzer.method_calls
    result["unsafe_patterns"] = analyzer.unsafe_patterns
    result["defined_methods"] = analyzer.defined_methods
    return result


if __name__ == "__main__":
    if len(sys.argv) != 2:
        print(json.dumps({"error": "usage: analyze_script.py <script_path>"}))
        sys.exit(0)
    try:
        with open(sys.argv[1], "r") as f:
            source = f.read()
    except OSError as e:
        print(json.dumps({"error": str(e)}))
        sys.exit(0)
    print(json.dumps(analyze(source, filename=sys.argv[1])))
