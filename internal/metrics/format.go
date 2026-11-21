package metrics

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// FormatDogStatsD validates samples against the allowlist and returns
// DogStatsD gauge lines: "<metric>:<value>|g|#<k1>:<v1>,<k2>:<v2>".
// Non-allowlisted samples are dropped.
func FormatDogStatsD(samples []Sample) []string {
	lines := make([]string, 0, len(samples))
	for _, s := range samples {
		def, tags, err := ValidateSample(s)
		if err != nil {
			continue
		}
		var b strings.Builder
		b.WriteString(def.DogStatsD)
		b.WriteByte(':')
		b.WriteString(formatFloat(s.Value))
		b.WriteString("|g|#")
		for i, t := range tags {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(t.Key)
			b.WriteByte(':')
			b.WriteString(t.Value)
		}
		lines = append(lines, b.String())
	}
	return lines
}

// BatchDogStatsD packs newline-delimited DogStatsD lines into UDP payloads no
// larger than maxBytes (defaulting to MaxDatagramBytes when maxBytes <= 0).
func BatchDogStatsD(lines []string, maxBytes int) [][]byte {
	if maxBytes <= 0 {
		maxBytes = MaxDatagramBytes
	}
	var packets [][]byte
	var buf []byte
	for _, line := range lines {
		if line == "" {
			continue
		}
		needed := len(line)
		if len(buf) > 0 {
			needed++ // separating '\n'
		}
		if len(buf) > 0 && len(buf)+needed > maxBytes {
			packets = append(packets, buf)
			buf = nil
		}
		if len(buf) > 0 {
			buf = append(buf, '\n')
		}
		buf = append(buf, line...)
	}
	if len(buf) > 0 {
		packets = append(packets, buf)
	}
	return packets
}

// FormatPrometheus validates samples against the allowlist and renders
// Prometheus text exposition format 0.0.4 with deterministic metric and series
// ordering. Non-allowlisted samples are dropped.
func FormatPrometheus(samples []Sample) string {
	type entry struct {
		labels string
		value  string
	}
	grouped := make(map[string][]entry, len(AllowlistOrder))
	for _, s := range samples {
		def, tags, err := ValidateSample(s)
		if err != nil {
			continue
		}
		var lb strings.Builder
		lb.WriteByte('{')
		for i, t := range tags {
			if i > 0 {
				lb.WriteByte(',')
			}
			lb.WriteString(t.Key)
			lb.WriteString(`="`)
			lb.WriteString(t.Value)
			lb.WriteByte('"')
		}
		lb.WriteByte('}')
		grouped[def.DogStatsD] = append(grouped[def.DogStatsD], entry{
			labels: lb.String(),
			value:  formatFloat(s.Value),
		})
	}
	var out strings.Builder
	for _, def := range AllowlistOrder {
		entries := grouped[def.DogStatsD]
		if len(entries) == 0 {
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].labels < entries[j].labels })
		fmt.Fprintf(&out, "# HELP %s %s\n", def.Prometheus, def.Help)
		fmt.Fprintf(&out, "# TYPE %s gauge\n", def.Prometheus)
		for _, e := range entries {
			fmt.Fprintf(&out, "%s%s %s\n", def.Prometheus, e.labels, e.value)
		}
	}
	return out.String()
}

func formatFloat(v float64) string {
	if v == 0 {
		return "0"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}
