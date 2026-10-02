"""Read-only Blender preflight using the separately installed Nanolathe exporter.

blender --background --disable-autoexec unit.blend --python-exit-code 1 \
  --python validate_blend.py -- --exporter /path/to/exporter \
  --profile unit.profile.json --report /path/to/preflight.json
"""

import argparse
from collections import Counter
import json
from pathlib import Path
import sys

import bpy


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--exporter", required=True, type=Path)
    parser.add_argument("--profile", type=Path)
    parser.add_argument("--report", required=True, type=Path)
    args = parser.parse_args(sys.argv[sys.argv.index("--") + 1:] if "--" in sys.argv else [])
    exporter = args.exporter.expanduser().resolve()
    core_path = exporter / "addon/nanolathe_export/core.py"
    if not core_path.is_file():
        parser.error(f"Nanolathe exporter core not found at {core_path}")
    sys.path.insert(0, str(exporter / "addon"))
    from nanolathe_export import bl_info, core
    if Path(core.__file__).resolve() != core_path:
        parser.error("A different Nanolathe exporter is already loaded; restart Blender "
                     "with --factory-startup before loading the blend, or select that exporter")

    profile = core.load_profile(bpy.context.scene, args.profile)
    diagnostics, objects = core.validate(bpy.context.scene, profile)
    counts = Counter()
    parts = []
    for obj in objects:
        faces = list(obj.data.polygons) if obj.type == "MESH" else []
        team_faces = 0
        for face in faces:
            n = len(face.vertices)
            counts["quads" if n == 4 else "triangles" if n == 3 else "other_faces"] += 1
            mat = obj.data.materials[face.material_index] if face.material_index < len(obj.data.materials) else None
            binding = mat.get("nanolathe_team_texture", "colorsmd" if mat.name == "TEAM_COLOR" else "") if mat else ""
            if binding == "colorsmd":
                team_faces += 1
        counts["team_faces"] += team_faces
        parts.append({"name": obj.name, "parent": obj.parent.name if obj.parent else None,
                      "type": obj.type, "faces": len(faces), "team_faces": team_faces})
    errors = [d for d in diagnostics.items if d["severity"] == "ERROR"]
    report = {
        "scope": "Exporter authoring preflight only; no native export or runtime claim",
        "blend": bpy.data.filepath,
        "exporter_root": str(exporter),
        "exporter_version": ".".join(map(str, bl_info["version"])),
        "exporter_profile_schema": core.PROFILE_SCHEMA,
        "unit": profile.get("unit"),
        "collection": profile.get("collection"),
        "passed": not errors,
        "counts": {key: counts[key] for key in ("quads", "triangles", "other_faces", "team_faces")},
        "selection_plate_included_in_counts": False,
        "pieces": parts,
        "diagnostics": diagnostics.items,
    }
    args.report.parent.mkdir(parents=True, exist_ok=True)
    args.report.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({"passed": not errors, "counts": report["counts"],
                      "errors": len(errors), "report": str(args.report)}))
    if errors:
        raise RuntimeError(f"Nanolathe preflight failed; inspect {args.report}")


if __name__ == "__main__":
    main()
