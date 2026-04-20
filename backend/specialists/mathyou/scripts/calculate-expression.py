#!/usr/bin/env python3
import ast
import json
import math
import operator
import sys

EXPRESSION = "__TOKEN_expression:string__"
SCRIPT_ID = "__SCRIPT_ID__"
TTY = "/dev/ttyAMA0"

BINARY_OPS = {
    ast.Add: operator.add,
    ast.Sub: operator.sub,
    ast.Mult: operator.mul,
    ast.Div: operator.truediv,
    ast.FloorDiv: operator.floordiv,
    ast.Mod: operator.mod,
    ast.Pow: operator.pow,
}

UNARY_OPS = {
    ast.UAdd: operator.pos,
    ast.USub: operator.neg,
}


def emit(obj: dict) -> None:
    with open(TTY, "w") as f:
        f.write(json.dumps(obj) + "\n")
        f.flush()


def is_unresolved(value: str) -> bool:
    return value.startswith("__TOKEN_")


def eval_node(node: ast.AST) -> float:
    if isinstance(node, ast.Expression):
        return eval_node(node.body)
    if isinstance(node, ast.Constant) and isinstance(node.value, (int, float)):
        return float(node.value)
    if isinstance(node, ast.UnaryOp) and type(node.op) in UNARY_OPS:
        return UNARY_OPS[type(node.op)](eval_node(node.operand))
    if isinstance(node, ast.BinOp) and type(node.op) in BINARY_OPS:
        return BINARY_OPS[type(node.op)](eval_node(node.left), eval_node(node.right))
    raise ValueError("unsupported expression")


def calculate(expression: str):
    parsed = ast.parse(expression, mode="eval")
    result = eval_node(parsed)
    if math.isfinite(result):
        # Keep integers clean when possible.
        if result.is_integer():
            return int(result)
        return result
    raise ValueError("non-finite result")


if __name__ == "__main__":
    if is_unresolved(EXPRESSION):
        emit({"type": "result", "script": SCRIPT_ID, "ok": False, "output": json.dumps({"error": "expression token was not replaced"})})
        sys.exit(1)

    try:
        expr = EXPRESSION.strip()
        if not expr:
            raise ValueError("expression must not be empty")
        result = calculate(expr)
        emit({"type": "result", "script": SCRIPT_ID, "ok": True, "output": json.dumps({"expression": expr, "result": result})})
    except Exception as e:
        emit({"type": "result", "script": SCRIPT_ID, "ok": False, "output": json.dumps({"error": str(e)})})
        sys.exit(1)
