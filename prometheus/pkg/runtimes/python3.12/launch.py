import importlib.util
import json
import sys
import traceback

spec = importlib.util.spec_from_file_location("handler", "/app/handler.py")
handler = importlib.util.module_from_spec(spec)
spec.loader.exec_module(handler)

event = json.load(sys.stdin)
try:
    result = handler.handler(event)
    json.dump(result, sys.stdout)
except Exception:
    traceback.print_exc()
    sys.exit(1)
