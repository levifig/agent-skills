#!/usr/bin/env python3
"""
Check Python code style compliance.
Usage: check-python-style.py <file-or-directory>

Checks:
- Type hints presence
- Docstring presence for public functions/classes
- structlog usage patterns
- Bare except clauses
- Async patterns
"""

import ast
import sys
from pathlib import Path
from typing import Generator


class StyleChecker(ast.NodeVisitor):
    """AST visitor for checking style compliance."""

    def __init__(self, filepath: str):
        self.filepath = filepath
        self.errors: list[str] = []
        self.warnings: list[str] = []

    def visit_FunctionDef(self, node: ast.FunctionDef) -> None:
        self._check_function(node)
        self.generic_visit(node)

    def visit_AsyncFunctionDef(self, node: ast.AsyncFunctionDef) -> None:
        self._check_function(node)
        self.generic_visit(node)

    def _check_function(self, node) -> None:
        """Check function/method for style compliance."""
        # Skip private/dunder methods for some checks
        is_public = not node.name.startswith('_')
        is_dunder = node.name.startswith('__') and node.name.endswith('__')

        # Check return type hint (public functions only)
        if is_public and not is_dunder and node.returns is None:
            self.warnings.append(
                f"Line {node.lineno}: Function '{node.name}' missing return type hint"
            )

        # Check argument type hints
        for arg in node.args.args:
            if arg.arg != 'self' and arg.arg != 'cls' and arg.annotation is None:
                self.warnings.append(
                    f"Line {node.lineno}: Argument '{arg.arg}' in '{node.name}' missing type hint"
                )

        # Check docstring for public functions
        if is_public and not is_dunder:
            if not (node.body and isinstance(node.body[0], ast.Expr) and
                    isinstance(node.body[0].value, ast.Constant) and
                    isinstance(node.body[0].value.value, str)):
                self.warnings.append(
                    f"Line {node.lineno}: Public function '{node.name}' missing docstring"
                )

    def visit_ClassDef(self, node: ast.ClassDef) -> None:
        """Check class for style compliance."""
        is_public = not node.name.startswith('_')

        # Check docstring for public classes
        if is_public:
            if not (node.body and isinstance(node.body[0], ast.Expr) and
                    isinstance(node.body[0].value, ast.Constant) and
                    isinstance(node.body[0].value.value, str)):
                self.warnings.append(
                    f"Line {node.lineno}: Public class '{node.name}' missing docstring"
                )

        self.generic_visit(node)

    def visit_ExceptHandler(self, node: ast.ExceptHandler) -> None:
        """Check for bare except clauses."""
        if node.type is None:
            self.errors.append(
                f"Line {node.lineno}: Bare 'except:' clause (specify exception type)"
            )
        self.generic_visit(node)

    def visit_Call(self, node: ast.Call) -> None:
        """Check for logging patterns."""
        # Check for print() usage (should use logging)
        if isinstance(node.func, ast.Name) and node.func.id == 'print':
            self.warnings.append(
                f"Line {node.lineno}: Consider using structlog instead of print()"
            )

        # Check structlog usage has context fields
        if isinstance(node.func, ast.Attribute):
            if node.func.attr in ['info', 'warning', 'error', 'debug']:
                # Check if it's a logger call
                if isinstance(node.func.value, ast.Name) and 'log' in node.func.value.id.lower():
                    # Check for keyword arguments (context fields)
                    if not node.keywords:
                        self.warnings.append(
                            f"Line {node.lineno}: Logger call missing context fields"
                        )

        self.generic_visit(node)


def check_file(filepath: Path) -> tuple[list[str], list[str]]:
    """Check a single Python file."""
    try:
        content = filepath.read_text()
        tree = ast.parse(content)
    except SyntaxError as e:
        return [f"Syntax error: {e}"], []

    checker = StyleChecker(str(filepath))
    checker.visit(tree)

    return checker.errors, checker.warnings


def find_python_files(path: Path) -> Generator[Path, None, None]:
    """Find all Python files in directory."""
    if path.is_file() and path.suffix == '.py':
        yield path
    elif path.is_dir():
        for p in path.rglob('*.py'):
            # Skip common non-source directories
            if not any(part.startswith('.') or part in ['__pycache__', 'venv', '.venv', 'node_modules']
                       for part in p.parts):
                yield p


def main():
    if len(sys.argv) < 2:
        print("Usage: check-python-style.py <file-or-directory>")
        sys.exit(1)

    path = Path(sys.argv[1])
    if not path.exists():
        print(f"Error: {path} not found")
        sys.exit(1)

    total_errors = 0
    total_warnings = 0

    for filepath in find_python_files(path):
        errors, warnings = check_file(filepath)

        if errors or warnings:
            print(f"\n{filepath}:")

            for error in errors:
                print(f"  ✗ {error}")
                total_errors += 1

            for warning in warnings:
                print(f"  ⚠ {warning}")
                total_warnings += 1

    print(f"\n{'=' * 50}")
    print(f"Errors: {total_errors}, Warnings: {total_warnings}")

    sys.exit(1 if total_errors > 0 else 0)


if __name__ == '__main__':
    main()
