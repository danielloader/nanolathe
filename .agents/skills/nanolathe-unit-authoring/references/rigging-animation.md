# Piece hierarchy, aiming and basic animation

Read when splitting a model or creating actions. These are original-asset
authoring conventions and the current rigid exporter's limits, not claims that
every retail unit uses the same rig.

## Plan stable mechanical ownership

Choose the minimum useful pieces for independent motion. A tank can use:

```text
base
├── hull
│   └── turret       full-circle yaw
│       └── sleeve   pitch at the trunnion
│           └── barrel   recoil along the firing axis
│               └── flare   muzzle/query origin
├── trackl
└── trackr
```

For a walker, use a stable aim mount and a rotating torso/weapon assembly plus
separate leg joints. Put every part that must turn with a turret/torso underneath
it, including attached sensors and weapons. Give independently pitching or
recoiling weapons their own pieces. Fixed hull fittings stay on the hull.
Join decorative meshes that always move together without moving their pivot.

Place `flare` at the actual barrel exit and make it inherit yaw, pitch and recoil.
Verify `QueryPrimary` returns it and `AimFromPrimary` returns the intended aim
origin. A locator alone is not a visual muzzle flash; effects need their existing
weapon/script path. Check projectile birth beyond the muzzle and outside armor
through the allowed aim sweep, not only at rest.

## Full-circle aiming and exceptions

For the user's armed ground-unit workflow, design turret/torso yaw for a full
360 degrees unless the concept explicitly has a fixed or restricted weapon.
Leave `aim.yaw_limits_degrees` absent for unrestricted yaw; `[-180,180]` is **not**
the way to request it in exporter 0.1.2, whose optional bound intervals must span
less than 180 degrees. Verify side/rear acquisition, signed angle wrap and the
path between poses, including pitch and full recoil clearance.

Aircraft, fixed guns and other exceptions require the actual weapon family and
chassis turning behavior. Read the existing FBI/weapon/script setup and the
owning [weapon](../../../../docs/DESIGN_WEAPONS_PROJECTILES.md) and
[movement](../../../../docs/DESIGN_MOVEMENT_PATH.md) contracts. Do not add an
invisible omnidirectional turret merely to satisfy exporter roles, clamp a visual
gun while admitting backward shots, or assume a rejected aim turns the chassis.

The current exporter requires separate globally aligned yaw → pitch → muzzle
descendants and a primary weapon role set. A unit outside that template may need
its existing authored COB integration or separately scoped exporter support.
Document the gap; do not pretend the ground-vehicle template covers it.

Pitch bounds are signed callback degrees containing rest zero; positive means
elevation. Choose any restriction from swept mechanical clearance and the unit's
intended targeting, not from example numbers. A rejected aim returns not-ready;
it does not automatically clear the engine's request latch or arrange a retry.
Test reacquisition/reissue if restrictions are used.

`AimPrimary` must reach both requested angles before granting readiness and hold
the aim through firing delay. New aims interrupt older aims/restores.
`FirePrimary` runs after shot admission, so it cannot veto a projectile. Generated
recoil and delayed return-to-rest need interruption and repeated-fire checks.
The exporter generates those callbacks; Blender aim-preview actions do not drive
runtime target tracking. Preserve unrelated custom script callbacks when updating
an existing unit; generated script replacement needs a behavior comparison.

## Rigid action subset

Use Euler object rotations and local location keyframes, one slot per action.
Use named actions `clipname::piecename` for explicit `source="actions"` clips,
or the profile's action-name map. Save a documented rest frame (commonly 1) with
unit scale and stable origins. Use a frame rate explicit in the source, normally
30 fps for a 30 Hz engine. Sampled intervals must map to at least one engine tick.

For a simple tank, start with runtime yaw, pitch, recoil and restored rest.
Static tracks/wheels are an honest first implementation. Do not promise animated
treads from texture scrolling: the current exporter does not support that path.
Add locomotion only through a verified supported animation/script route.

Walk clips own only their declared pieces/lanes; weapon yaw, pitch and recoil
must not have competing walk writers. Translation bob on an aim ancestor is
supported; rotational bob/roll on that ancestor invalidates chassis-relative
aim and is rejected. Put visual rocking on a separate shell or keep the aim
mount independent. Do not solve that mismatch by changing engine angles ad hoc.

Exporter 0.1.2 supports up to three phase-aligned walk speed tiers with equal
segment counts, 2–120 segments per clip, local translation/Euler sampling and
loop closure. It rejects bones/skinning, drivers, active NLA, animated scale,
F-curve modifiers and ambiguous 180-degree sampled jumps. Increase sample density
for large turns; full repeated spins are outside this walk subset.

`Create` restores rest, `StartMoving` starts the loop and `StopMoving` restores
its owners. Interpolated stop may move feet through the ground; `stop_mode="snap"`
commits exact rest immediately but visibly snaps. Choose and report the policy,
then inspect stops at different gait phases and moving aim. Do not present these
as an authored contact-preserving start/stop transition. Separate idle/create/
start/stop clips, arbitrary animation state machines and multiple generated
weapon profiles are outside the current template.

Keep a compact profile alongside the `.blend` with unit ID, collection, rest
frame, scale, selection extents, tile size, explicit aim/from/muzzle roles,
optional recoil and walk owners/clips. Use the actual exporter's schema and
existing unit settings; do not copy illustrative speeds, recoil distances or
pitch bounds into every unit.
