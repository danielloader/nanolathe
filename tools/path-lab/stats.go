package main

import (
	"fmt"
	"sort"
	"strings"
)

// tally is a bag of named counters with sums, merged across recordings. A
// key is "table|dim=value|dim=value".
type tally map[string]*cell

type cell struct {
	N   int64   `json:"n"`
	Sum float64 `json:"sum"`
}

func (t tally) add(key string, v float64) {
	c := t[key]
	if c == nil {
		c = &cell{}
		t[key] = c
	}
	c.N++
	c.Sum += v
}

func (t tally) merge(o tally) {
	for k, c := range o {
		d := t[k]
		if d == nil {
			d = &cell{}
			t[k] = d
		}
		d.N += c.N
		d.Sum += c.Sum
	}
}

func key(table string, kv ...string) string {
	var b strings.Builder
	b.WriteString(table)
	for i := 0; i+1 < len(kv); i += 2 {
		b.WriteByte('|')
		b.WriteString(kv[i])
		b.WriteByte('=')
		b.WriteString(kv[i+1])
	}
	return b.String()
}

// bucket names the first bound that v does not exceed.
func bucket(v float64, bounds ...float64) string {
	lo := 0.0
	for _, b := range bounds {
		if v <= b {
			return fmt.Sprintf("%05g-%g", lo, b)
		}
		lo = b
	}
	return fmt.Sprintf("%05g+", lo)
}

func yes(b bool) string {
	if b {
		return "y"
	}
	return "n"
}

// table returns the rows of one table, sorted by key.
func (t tally) table(name string) []string {
	var ks []string
	for k := range t {
		if strings.HasPrefix(k, name+"|") || k == name {
			ks = append(ks, k)
		}
	}
	sort.Strings(ks)
	return ks
}
