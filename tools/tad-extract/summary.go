package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type runInfo struct {
	Workers     int            `json:"workers"`
	Recordings  int            `json:"recordings_listed"`
	Extracted   int            `json:"extracted_this_run"`
	Seconds     float64        `json:"seconds"`
	MBRead      float64        `json:"mb_decoded_this_run"`
	MBPerSecond float64        `json:"mb_per_second"`
	Results     map[string]int `json:"results"`
	Finished    string         `json:"finished"`
}

// sourceSummary aggregates every extract of one source.
type sourceSummary struct {
	Recordings    int              `json:"recordings"`
	Status        map[string]int   `json:"status"`
	Failures      map[string]int   `json:"failures_by_reason,omitempty"`
	RawBytes      int64            `json:"raw_bytes"`
	ExtractBytes  int64            `json:"extract_bytes"`
	Players       int              `json:"players"`
	Starts        int              `json:"build_starts"`
	Finished      int              `json:"build_finished"`
	Died          int              `json:"build_died"`
	Named         map[string]int   `json:"named_by_table"`
	Unnamed       int              `json:"unnamed_recordings"`
	EnabledKeys   map[string]int   `json:"enabled_key_counts"`
	Recorders     map[string]int   `json:"recorder_strings"`
	StatusRecords map[string]int   `json:"status_records"`
	Decode        map[string]int64 `json:"decode_totals"`
	Extract       map[string]int64 `json:"extract_totals"`
	Subpackets    map[string]int64 `json:"subpackets"`
	UnknownIDs    map[string]int64 `json:"unknown_ids,omitempty"`
}

type summary struct {
	Run     runInfo                   `json:"last_run"`
	Sources map[string]*sourceSummary `json:"sources"`
	Total   *sourceSummary            `json:"total"`
}

func newSourceSummary() *sourceSummary {
	return &sourceSummary{
		Status: map[string]int{}, Failures: map[string]int{}, Named: map[string]int{},
		EnabledKeys: map[string]int{}, Recorders: map[string]int{}, StatusRecords: map[string]int{},
		Decode: map[string]int64{}, Extract: map[string]int64{}, Subpackets: map[string]int64{},
		UnknownIDs: map[string]int64{},
	}
}

// addNumbers adds the numeric fields of v (a struct marshalled to JSON) to
// the totals map, so new counters appear in the summary without listing them.
func addNumbers(dst map[string]int64, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return
	}
	for k, x := range m {
		if f, ok := x.(float64); ok && k != "lz_max_out" && k != "end_offset" {
			dst[k] += int64(f)
		}
	}
}

func (s *sourceSummary) add(ex *extract, extractBytes int64) {
	s.Recordings++
	s.Status[ex.Status]++
	switch {
	case ex.Error != "":
		s.Failures[failureReason(ex.Error)]++
	case ex.Status == "truncated" && ex.Decode != nil:
		s.Failures["truncated: "+ex.Decode.End]++
	}
	s.RawBytes += ex.SourceBytes
	s.ExtractBytes += extractBytes
	if ex.Naming.Table != "" {
		s.Named[ex.Naming.Table]++
	} else {
		s.Unnamed++
	}
	if ex.Setup != nil {
		s.EnabledKeys[strconv.Itoa(ex.Setup.UnitData.Enabled)]++
		s.Recorders[ex.Setup.Recorder]++
		for _, st := range ex.Setup.Status {
			if st.Error != "" {
				s.StatusRecords["error"]++
				continue
			}
			if st.Checksum {
				s.StatusRecords["checksum_ok"]++
			} else {
				s.StatusRecords["checksum_mismatch"]++
			}
			s.StatusRecords["record_len_"+strconv.Itoa(st.RecordLen)]++
			if st.Compressed {
				s.StatusRecords["compressed"]++
			}
			if st.Sequence == -1 {
				s.StatusRecords["sequence_minus_one"]++
			}
			if st.DPID == st.DPID2 {
				s.StatusRecords["dpid_repeat_agrees"]++
			}
		}
	}
	s.Players += len(ex.Players)
	for _, p := range ex.Players {
		for _, b := range p.Builds {
			s.Starts++
			if b.FinMs != nil {
				s.Finished++
			}
			if b.DiedMs != nil {
				s.Died++
			}
		}
	}
	if ex.Decode != nil {
		addNumbers(s.Decode, ex.Decode)
		for k, v := range ex.Decode.Subpackets {
			s.Subpackets[k] += v
		}
		for k, v := range ex.Decode.UnknownIDs {
			s.UnknownIDs[k] += int64(v)
		}
	}
	if ex.Extract != nil {
		addNumbers(s.Extract, ex.Extract)
	}
}

func (s *sourceSummary) merge(o *sourceSummary) {
	s.Recordings += o.Recordings
	s.RawBytes += o.RawBytes
	s.ExtractBytes += o.ExtractBytes
	s.Players += o.Players
	s.Starts += o.Starts
	s.Finished += o.Finished
	s.Died += o.Died
	s.Unnamed += o.Unnamed
	for _, p := range []struct{ d, s map[string]int }{
		{s.Status, o.Status}, {s.Failures, o.Failures}, {s.Named, o.Named},
		{s.EnabledKeys, o.EnabledKeys}, {s.Recorders, o.Recorders}, {s.StatusRecords, o.StatusRecords},
	} {
		for k, v := range p.s {
			p.d[k] += v
		}
	}
	for _, p := range []struct{ d, s map[string]int64 }{
		{s.Decode, o.Decode}, {s.Extract, o.Extract}, {s.Subpackets, o.Subpackets}, {s.UnknownIDs, o.UnknownIDs},
	} {
		for k, v := range p.s {
			p.d[k] += v
		}
	}
}

// failureReason drops file-specific detail so failures group by cause.
func failureReason(e string) string {
	for _, cut := range []string{"unsupported format version", "implausible sector count"} {
		if strings.Contains(e, cut) {
			return cut
		}
	}
	return e
}

// writeSummary aggregates every extract of the listed recordings into
// <out>/summary.json.
func writeSummary(out string, demos []demo, run runInfo) error {
	sm := summary{Run: run, Sources: map[string]*sourceSummary{}, Total: newSourceSummary()}
	for _, d := range demos {
		p := extractPath(out, d)
		ex, err := readExtract(p)
		if err != nil {
			continue
		}
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		s := sm.Sources[d.source]
		if s == nil {
			s = newSourceSummary()
			sm.Sources[d.source] = s
		}
		s.add(ex, fi.Size())
	}
	names := make([]string, 0, len(sm.Sources))
	for k := range sm.Sources {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		sm.Total.merge(sm.Sources[k])
	}
	b, err := json.MarshalIndent(sm, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "summary.json"), append(b, '\n'), 0o644)
}
