package audio

import (
	"errors"
	"reflect"
	"testing"
)

// Fresh open has distinct request and next seeds [07 R-FE-01 §6]. The
// production service binds its media once, so later configuration cannot
// overwrite an edited request. The files are authored probe placeholders.
func TestMusicOptionsFreshOpenRequest(t *testing.T) {
	for _, tracks := range []int{0, 3} {
		m := NewMusicController()
		m.SetRequestedTrack(2)
		m.Open(tracks)
		if m.RequestedTrack() != 1 || m.NextTrack() != 0 {
			t.Fatalf("tracks=%d request=%d next=%d", tracks, m.RequestedTrack(), m.NextTrack())
		}
	}
	s := NewService(musicFS(t, "1.wav", "2.wav", "3.wav"))
	s.ConfigureMusic(false)
	if s.Music.RequestedTrack() != 1 || s.Music.NextTrack() != 0 {
		t.Fatal("production media initialization lost the fresh-open seeds")
	}
	s.Music.SetRequestedTrack(3)
	s.ConfigureMusic(true)
	if s.Music.RequestedTrack() != 3 {
		t.Fatal("already-bound service reopened its controller")
	}
}

// The selector dispatches from controller status, preserves the independent
// request, and returns resulting next rather than assuming playback succeeded
// [07 R-FE-01 §6]. File failure retains Nanolathe's existing idle/error policy.
func TestMusicOptionsSelectTrack(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     StatusMode
		disabled   bool
		count      int
		argument   int
		fail       bool
		wantNext   int
		wantResult int
		wantStatus StatusMode
		wantPlays  []int
	}{
		{"stopped", StatusIdle, false, 3, 2, false, 2, 2, StatusIdle, nil},
		{"paused", StatusPaused, false, 3, 3, false, 3, 3, StatusPaused, nil},
		{"playing", StatusPlaying, false, 3, 2, false, 2, 2, StatusPlaying, []int{2}},
		{"disabled-stopped", StatusIdle, true, 3, 2, false, 2, 2, StatusIdle, nil},
		{"disabled-playing", StatusPlaying, true, 3, 2, false, 1, 1, StatusPlaying, nil},
		{"empty", StatusPlaying, false, 0, 2, false, 1, 0, StatusPlaying, nil},
		{"modulo-stopped", StatusIdle, false, 3, 5, false, 2, 2, StatusIdle, nil},
		{"modulo-playing", StatusPlaying, false, 3, 5, false, 2, 2, StatusPlaying, []int{2}},
		{"exact-multiple-stopped", StatusIdle, false, 3, 6, false, 0, 0, StatusIdle, nil},
		{"exact-multiple-playing", StatusPlaying, false, 3, 6, false, 2, 2, StatusPlaying, []int{2}},
		{"failed-submission", StatusPlaying, false, 3, 2, true, 2, 2, StatusIdle, []int{2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewMusicController()
			m.Open(3)
			m.Configure(ModeSequential, 0)
			m.SetRequestedTrack(3)
			// Model the independent controller status/enable/count boundary;
			// a disabled tick can leave status playing without a media request.
			m.status, m.musicEnabled, m.nextTrack = tc.status, !tc.disabled, 1
			m.SetNumTracks(tc.count)
			var plays []int
			polls := 0
			m.SetPlaybackPoll(func() bool { polls++; return false })
			m.openTrack = func(track int) (MusicPlayer, error) {
				plays = append(plays, track)
				if tc.fail {
					return nil, errors.New("authored unavailable track")
				}
				return nil, nil
			}
			result := m.SelectTrack(tc.argument)
			if result != tc.wantResult || m.NextTrack() != tc.wantNext || m.Status() != tc.wantStatus {
				t.Fatalf("result=%d next=%d status=%v", result, m.NextTrack(), m.Status())
			}
			if !reflect.DeepEqual(plays, tc.wantPlays) || m.RequestedTrack() != 3 {
				t.Fatalf("plays=%v request=%d", plays, m.RequestedTrack())
			}
			if len(tc.wantPlays) == 0 && polls != 0 {
				t.Fatalf("nonplaying selector queried media %d times", polls)
			}
			if tc.fail && m.mediaError == nil {
				t.Fatal("file-open failure was lost")
			}
		})
	}
	var absent *Controller
	if absent.SelectTrack(1) != 0 {
		t.Fatal("absent controller selected a track")
	}
}

// Explicit page updates must query media rather than reusing controller
// status, and must preserve idle's status-before-query ordering. They do not
// advance the timer pump [03 R-AUD-01 §4][07 R-FE-01 §6].
func TestMusicOptionsUpdateNow(t *testing.T) {
	m := NewMusicController()
	m.Open(3)
	m.Configure(ModeSequential, 0)
	m.status, m.nextTrack = StatusPlaying, 1
	var plays []int
	m.openTrack = func(track int) (MusicPlayer, error) {
		plays = append(plays, track)
		return nil, nil
	}
	polls := 0
	m.SetPlaybackPoll(func() bool { polls++; return false })
	m.UpdateNow()
	if !reflect.DeepEqual(plays, []int{2}) || polls == 0 {
		t.Fatalf("status substituted for device query: plays=%v polls=%d", plays, polls)
	}

	m.Configure(ModeIdle, 0)
	m.SetPlaybackPoll(func() bool {
		if m.Status() != StatusIdle {
			t.Fatal("idle queried before clearing status")
		}
		return true
	})
	m.UpdateNow()
	if m.NextTrack() != 1 || m.Status() != StatusIdle {
		t.Fatal("idle playing reply did not stop/reset")
	}

	m.Configure(ModeSequential, 0)
	clockReads := 0
	m.SetPresentationClock(func() uint32 { clockReads++; return 100 })
	m.timers[0] = musicTimer{kind: musicDelay, period: 1, remaining: 1}
	before := m.timers
	clockReads = 0
	m.SetPlaybackPoll(func() bool { return true })
	m.UpdateNow()
	if clockReads != 0 || m.timers != before {
		t.Fatal("explicit update serviced presentation timers")
	}
	var absent *Controller
	absent.UpdateNow()
}

func TestMusicOptionsUpdateNowEarlyExits(t *testing.T) {
	for _, tc := range []struct {
		name    string
		count   int
		status  StatusMode
		mode    PlayMode
		desired int
	}{
		{"no-tracks", 0, StatusPlaying, ModeSequential, 0},
		{"paused", 3, StatusPaused, ModeSequential, 0},
		{"silence-before-pause", 3, StatusPaused, ModeSequential, 4},
		{"already-idle", 3, StatusIdle, ModeIdle, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewMusicController()
			m.Open(tc.count)
			m.Configure(tc.mode, tc.desired)
			m.status = tc.status
			m.SetPlaybackPoll(func() bool { t.Fatal("early exit queried media"); return false })
			m.UpdateNow()
			if tc.desired == 4 && m.Status() != StatusIdle {
				t.Fatal("paused status bypassed the silence stop")
			}
		})
	}
}
