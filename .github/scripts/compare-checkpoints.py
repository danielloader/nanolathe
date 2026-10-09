#!/usr/bin/env python3
"""Compare the authored scene's native Go-test evidence (multiplayer M3-C8)."""
import json
from pathlib import Path
import sys

root = Path(sys.argv[1])
expected = {
    "checkpoints-darwin-arm64-native",
    "checkpoints-linux-amd64-v1",
    "checkpoints-linux-amd64-v3",
    "checkpoints-windows-amd64-v1",
}
files = sorted(root.glob("*/checkpoints.json"))
if {p.parent.name for p in files} != expected:
    raise SystemExit("missing or unexpected native checkpoint evidence")
reference = None
for path in files:
    records = []
    passed = set()
    for line in path.read_text(encoding="utf-8").splitlines():
        event = json.loads(line)
        if event.get("Action") == "fail":
            raise SystemExit(f"failed native test in {path}")
        if event.get("Action") == "pass":
            passed.add(event.get("Test", ""))
        output = event.get("Output", "")
        if "portable-v1 " in output:
            records.append(output.split("portable-v1 ", 1)[1].strip())
    modes = {"TestCheckpointPortableScript/" + mode for mode in ("strict-3.1", "modern", "community-3.9")}
    if not modes <= passed or not records:
        raise SystemExit(f"incomplete native scene in {path}")
    if reference is None:
        reference = records
    elif records != reference:
        for index, (want, got) in enumerate(zip(reference, records)):
            if want != got:
                raise SystemExit(f"{path.parent.name} differs at evidence row {index}:\nreference: {want}\nreceived:  {got}")
        raise SystemExit(f"{path.parent.name} differs in evidence length")
    print(f"{path.parent.name}: {len(records)} identical identity/checkpoint/owner rows")
