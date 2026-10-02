//go:build retail

package headless

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
)

// The gameplay rule sets are locked here by fingerprint, one checked-in
// constant per scene and mode. The point of the lock is asymmetric: a Modern
// rule may be changed on purpose, but it must not move the Strict 3.1 baseline
// as a side effect, and neither may drift by accident. Every constant below is
// therefore a behavioural claim, and a diff that changes one is claiming the
// simulation now behaves differently.
//
// The fingerprint is the deliberately partial digest of
// docs/DESIGN_RUNTIME_DETERMINISM.md §4: equal digests do not prove equal
// futures, and a change confined to excluded state (projectiles, AI tasks,
// visibility, construction internals) will not show here. It is a regression
// tripwire, not a parity proof.
//
// HOW TO UPDATE A CONSTANT. Run the failing subtest, take the reported value,
// and say in the commit message which behaviour changed and why the new value
// is correct. A Strict constant moving is a retail-baseline change and needs
// the research citation that justifies it; a Modern constant moving is an
// approved-policy change and needs its design-document section. Updating a
// value with "fingerprint moved" as the whole explanation defeats the guard.
//
// Strict values retain the pre-Community retail baseline. Community and Modern
// values were recorded after DESIGN_COMMUNITY_PATCH §4.1 entry parameters:
// 1500 units per player, 66650 path steps, and larger effect/projectile pools.
// Unit identities change at composition and the path allowance changes the
// computer player's progress. The long ashap runs now finish before the 54000
// tick bound; both their terminal tick and fingerprint are locked. The battle
// fixture warm/final Community and Modern locks also include CP-DMG-2: lower
// unit indices take contested cells, changing movement and collision victims.
// Community's long lock additionally includes CP-CON-1's inclusive blocked-site
// limit of 20 rather than 10. Modern retains its existing limit of 10 under D3
// (DESIGN_COMMUNITY_PATCH §4.3, §11), restoring the pre-kickout terminal lock.
// The 6000-tick and benchmark locks remain unchanged.
//
// Modern re-route staggering (DESIGN_MOVEMENT_PATH "Modern re-route
// staggering") moves the Modern long lock and the benchmark warm/final locks:
// re-routes arrive 0–7 ticks later, so the long run no longer ends before
// its 54000-tick bound. The Modern benchmark initial and 6000-tick locks, and
// every Strict and Community lock, are unchanged by it.
//
// Modern group-order spreading (DESIGN_MOVEMENT_PATH "Modern group-order
// spreading") moves the Modern benchmark warm/final locks again: the
// computer players' group orders admit their farther members one or two
// ticks later. The ashap scene forms no group of sixteen same-tick first
// requests (its largest is seven), so its Modern locks are unchanged, as are
// the Modern benchmark initial lock and every Strict and Community lock.
//
// Modern bounded path work (DESIGN_MOVEMENT_PATH "Modern bounded path work")
// moves the Modern benchmark warm/final locks once more: a player's carried
// search work is capped at four shares and a whole sweep that admits nothing
// ends its polling for the call, which changes the poll cursor's position and
// so the order later requests are admitted in. The ashap scene's Modern locks,
// the benchmark initial lock and every Strict and Community lock are
// unchanged by it.
//
// Modern allied pass-through (DESIGN_MOVEMENT_PATH "Modern allied
// pass-through") moves the Modern benchmark warm/final locks: the computer
// armies' opposed movers now pass through each other mid-route instead of
// blocking. The ashap locks, the benchmark initial lock and every Strict and
// Community lock are unchanged by it.
//
// Retail's `MobileBuild` weapon-slot release [04 R-ORD-01 §5] moves the Strict
// and Community ashap 6000-tick locks and nothing else: at that tick the Core
// commander is building with slots 0 and 2 taken from autonomy, and setting
// that one bit back reproduces the previous values exactly. The trajectories
// do not diverge, so the 54000-tick and benchmark locks hold; the Modern
// commander is still approaching its site at tick 6000, so its lock holds too.
//
// Retail runs the goal installer's acceptance rule for the mobile-build
// rectangle too [04 R-PATH-01 §8][04 R-PATH-01 §13]: a builder walks the
// synthetic straight line at the rectangle's goal point from the order's own
// tick, and a stale last-request tick is zeroed, where it used to stand until
// the search published. That moves every Strict, Community and Modern lock but
// the benchmark initial ones, and the Community and Modern long runs now end at
// ticks 43530 and 40710. Of those, the installer's age test being inclusive at
// ten ticks ([04 R-PATH-01 §8] step 6) alone moves the Community and Modern
// benchmark warm and final locks and the Modern long run (which would end at
// 35580 without it); no Strict lock and no Community ashap lock sees it.
//
// Retail's front-record pump [04 §3.3][04 R-ORD-01 §0] moves the Strict ashap
// 54000-tick lock and nothing else. The computer player queues work on
// products still being built, and the construction step used to advance a
// queued MobileBuild behind the unfinished product's GetBuilt head into its
// approach phase. It now waits at phase 0 until GetBuilt completes. At that
// tick the approach starts either way, so the run does not diverge: draw
// counts, units created, live units and orders submitted are equal, and the
// Strict hashes at every 6000-tick step to 36000 and at 48000 are unchanged.
// Only the queued record's own state differs at tick 54000 (and at 42000).
// The Community and Modern locks do not move. (Measured on top of the
// build-walk start above; before it, this moved the Modern long lock instead.)
//
// Retail's allocator tail [04 §2.3b][04 R-MOV-01 §5] moves every lock. A
// mobile unit is now created facing its authored `buildangle`, so both
// commanders start facing north instead of the drawn heading's south, and
// every created mover is grounded or floated at creation. The benchmark scene
// composes differently from its first frame. The ashap trajectories diverge
// from the commanders' first turns: the computer player builds at the same
// rate as before (units created at 12000/18000/24000 ticks within two of the
// previous run in every set) but its first attack reaches the idle commander
// at a different time. Strict now ends at tick 28680 instead of reaching its
// 54000-tick bound, Community at 30660 and Modern at 45390.
//
// Modern traffic (DESIGN_MOVEMENT_PATH "Modern traffic") moves every Modern
// lock but the benchmark initial one. Friendly units no longer share cells:
// allied pass-through, jam release and pocket release are retired from
// Modern. Ground movers steer round what is ahead of them, routes are pulled
// taut and searched with Modern's own weights, a follower asks again after
// half a second, and the units of a group are given places. The Modern long
// run after that adoption ended at tick 40470. No Strict or Community lock moved. The build
// before the adoption, playing these scenes under the pathfinding
// laboratory's candidate rule set, reached the same values bit for bit.
// Modern friendly/feature shot admission (DESIGN_WEAPONS_PROJECTILES §2.3.2)
// changes the combat warm/final locks and the long Modern ending: refused
// launches and cancelled pellets retain their RNG/resource effects. The
// initial and 6000-tick Modern states, and every Strict/Community lock, stay
// unchanged. The long Modern battle now ends at tick 40830.
//
// Preserving MobileBuild's follower through placement [05 R-WORK-01 §14]
// and using the stored point count for pruning/retry arming [04 R-MOV-03 §2]
// correct the shared retail mechanics. An inactive route no longer causes an
// extra goal install or an unblocked retry, while a reached stale point still
// prunes. This moves the Community benchmark final lock, the Modern warm/final
// locks and its long ending (now 42600). Every Strict lock, every initial lock,
// the Community ashap/warm locks and the Modern 6000-tick lock stay unchanged.
// Modern traffic and its fifteen-tick retry admission delay are preserved
// (DESIGN_MOVEMENT_PATH "Modern prompt re-routing").
//
// Retail's ground MobileBuild waits for the script-owned INBUILDSTANCE before
// health, resources and spray progress [04 R-ORD-01 §5][05 R-P0-06 §1]. This
// moves every ashap lock and no benchmark lock. The Core commander's first
// frame now holds with zero health until its script writes readiness, where
// the previous handler worked with stance clear. Those construction timings
// change resource and RNG history, and then the computer player's trajectory.
// Strict and Community now reach the 54000-tick ceiling with ongoing
// production; Modern ends at 47940. An overlay restoring only the preceding
// construction handler reproduces all six old ashap hashes and terminal ticks.
// Both candidate trajectories repeated identically; every benchmark initial,
// warm and final lock remains unchanged. This is a shared retail correction,
// with no new Modern policy.
//
// Correcting Modern's idle batching and arrival-place swaps moves its long
// ending to 49380 and its benchmark warm/final locks. Idle batching now stops
// before the poll that proves a full sweep, preserving individual polling's
// later admission order (DESIGN_MOVEMENT_PATH "Modern bounded path work").
// Crossed arrival places exchange only when each remains passable under its
// recipient's movement profile and knowledge ("Modern arrival places").
// A diagnostic build with only these two prior implementations reproduces
// every old Modern lock, isolating their effects from the other review fixes.
// Every Strict and Community lock, the Modern initial composition and its
// 6000-tick ashap lock remain unchanged.
//
// Modern firing positions (DESIGN_UNITS_ORDERS_COB "Modern firing positions")
// move obstructed ground attackers onto locally clear lanes. This changes
// Modern's benchmark final state and its long battle ending (now 49350), while
// its initial, warm and 6000-tick states and all Strict/Community locks remain
// unchanged. The captured Flash/Weasel regression isolates the new behavior:
// disabling only repositioning leaves the Flash parked behind the rock.
const (
	lockAshapMap                   = "ashap plateau"
	lockAshapSeed           uint32 = 7
	lockAshapUnitLimit             = 250 // Strict setting; Community's table overrides it.
	lockDifficulty                 = 1
	lockAshapStrict6000            = "partial-v1:aa2d3cb661ac26c4"
	lockAshapCommunity6000         = "partial-v1:abd94817ece852a4"
	lockAshapModern6000            = "partial-v1:a2c6fc1f6d2e6836"
	lockAshapStrict54000           = "partial-v1:24da164ea104466e"
	lockAshapCommunity54000        = "partial-v1:a76a48f358418a68"
	lockAshapModern54000           = "partial-v1:c68fedd7da4f36d0"
	lockAshapStrictEnd      uint32 = 54000
	lockAshapCommunityEnd   uint32 = 54000
	lockAshapModernEnd      uint32 = 49350

	lockBenchSeed             uint32 = 7
	lockBenchWarmupTicks             = 600
	lockBenchTotalTicks              = 1500
	lockBenchStrictInitial           = "partial-v1:bf488aacf042d582"
	lockBenchCommunityInitial        = "partial-v1:55165c066f8b6eaa"
	lockBenchModernInitial           = "partial-v1:55165c066f8b6eaa"
	lockBenchStrictWarm              = "partial-v1:5afe0089d8db02b8"
	lockBenchCommunityWarm           = "partial-v1:32013bdf5ea1c7ef"
	lockBenchModernWarm              = "partial-v1:ad31aa25d9f87893"
	lockBenchStrictFinal             = "partial-v1:2ccfe9ca8e307df2"
	lockBenchCommunityFinal          = "partial-v1:67c87da793a68133"
	lockBenchModernFinal             = "partial-v1:3854f2d414673aa0"
)

// TestStrictFingerprintIsLocked holds the retail baseline. Nothing in a Modern
// rule set may move any value here.
func TestStrictFingerprintIsLocked(t *testing.T) {
	runFingerprintLock(t, gameplay.Strict31, lockAshapStrict6000, lockAshapStrict54000, lockAshapStrictEnd, lockBenchStrictInitial, lockBenchStrictWarm, lockBenchStrictFinal)
}

// TestCommunityFingerprintIsLocked holds the approved mainline feature table.
func TestCommunityFingerprintIsLocked(t *testing.T) {
	runFingerprintLock(t, gameplay.Community39, lockAshapCommunity6000, lockAshapCommunity54000, lockAshapCommunityEnd, lockBenchCommunityInitial, lockBenchCommunityWarm, lockBenchCommunityFinal)
}

// TestModernFingerprintIsLocked holds the approved Modern policy set the same
// way, so an unintended change to a Modern rule is as loud as a change to the
// retail path [I11].
func TestModernFingerprintIsLocked(t *testing.T) {
	runFingerprintLock(t, gameplay.Modern, lockAshapModern6000, lockAshapModern54000, lockAshapModernEnd, lockBenchModernInitial, lockBenchModernWarm, lockBenchModernFinal)
}

// The combat scene distinguishes each reserved set. This catches a disabled
// Community layer or a Modern policy silently absorbed by its base.
func TestLockedScenesDiscriminateTheRuleSets(t *testing.T) {
	if lockBenchStrictFinal == lockBenchCommunityFinal || lockBenchCommunityFinal == lockBenchModernFinal || lockBenchStrictFinal == lockBenchModernFinal {
		t.Fatal("combat scene no longer distinguishes all three reserved sets")
	}
}

// runFingerprintLock runs both locked scenes under mode and compares each
// fingerprint with its constant. Elapsed times are logged, never asserted: the
// test is a value comparison and must pass identically on a loaded machine.
func runFingerprintLock(t *testing.T, mode gameplay.Mode, want6000, want54000 string, wantEnd uint32, wantBenchInitial, wantBenchWarm, wantBenchFinal string) {
	catalog, fs := retailcat.Shared(t)

	for _, scene := range []struct {
		ticks uint32
		end   uint32
		want  string
	}{
		{ticks: 6000, end: 6000, want: want6000},
		{ticks: 54000, end: wantEnd, want: want54000},
	} {
		t.Run(fmt.Sprintf("ashap-%d", scene.ticks), func(t *testing.T) {
			if testing.Short() && scene.ticks > 6000 {
				t.Skip("long fingerprint trajectory: run tools/check-retail --full")
			}
			start := time.Now()
			report, err := RunWithContent(Request{
				Gameplay:       mode,
				Map:            lockAshapMap,
				Difficulty:     lockDifficulty,
				SimulationSeed: lockAshapSeed,
				CRTSeed:        lockAshapSeed,
				TickLimit:      scene.ticks,
				UnitLimit:      lockAshapUnitLimit,
			}, fs, catalog)
			// A scene either reaches its bound or ends the battle at its locked tick.
			if err != nil && !errors.Is(err, ErrTickLimit) {
				t.Fatalf("%s %q for %d ticks: %v", mode, lockAshapMap, scene.ticks, err)
			}
			if report.Tick != scene.end {
				t.Errorf("%s %q stopped at tick %d, want %d: the locked constant describes the full run", mode, lockAshapMap, report.Tick, scene.end)
			}
			if report.StateHash != scene.want {
				t.Errorf("%s %q after %d ticks fingerprints %s, want the locked %s: a diff that intends this must say which behaviour changed",
					mode, lockAshapMap, scene.ticks, report.StateHash, scene.want)
			}
			t.Logf("%s %q %d ticks: %s in %s", mode, lockAshapMap, report.Tick, report.StateHash, time.Since(start).Round(time.Millisecond))
		})
	}

	start := time.Now()
	opts := SimBenchOptions{
		Gameplay:   mode,
		Map:        SimBenchDefaultMap,
		Seed:       lockBenchSeed,
		Difficulty: lockDifficulty,
		UnitLimit:  SimBenchDefaultUnitLimit,
	}
	// ComposeSimBenchBattle is the benchmark's fingerprint-only path: it
	// builds the session and places the three armies without running a tick or
	// measuring anything, so this lock shares the scene with tools/sim-bench
	// and reads no clock the simulation can see.
	composed, scene, err := ComposeSimBenchBattle(opts, fs, catalog)
	if err != nil {
		t.Fatalf("%s compose benchmark scene: %v", mode, err)
	}
	if composed.InitialFingerprint != wantBenchInitial {
		t.Errorf("%s benchmark scene composes to %s, want the locked %s: the scene itself changed, so every later constant here is about a different workload",
			mode, composed.InitialFingerprint, wantBenchInitial)
	}
	if scene.Version != SimBenchSceneVersion {
		t.Fatalf("%s benchmark scene is version %d, want %d", mode, scene.Version, SimBenchSceneVersion)
	}
	sess := composed.Session
	for tick := 1; tick <= lockBenchTotalTicks; tick++ {
		simBenchStep(sess)
		if tick != lockBenchWarmupTicks && tick != lockBenchTotalTicks {
			continue
		}
		hash, hashErr := sess.PartialStateFingerprint()
		if hashErr != nil {
			t.Fatalf("%s benchmark fingerprint at step %d: %v", mode, tick, hashErr)
		}
		want := wantBenchWarm
		if tick == lockBenchTotalTicks {
			want = wantBenchFinal
		}
		if hash != want {
			t.Errorf("%s benchmark scene after %d steps fingerprints %s, want the locked %s: a diff that intends this must say which behaviour changed",
				mode, tick, hash, want)
		}
		t.Logf("%s benchmark scene %d steps: %s in %s", mode, tick, hash, time.Since(start).Round(time.Millisecond))
	}
}
