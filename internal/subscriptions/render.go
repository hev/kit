package subscriptions

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// StateText is the human form of a login state.
func StateText(state string) string {
	switch state {
	case SignedIn:
		return "signed in"
	case NotSignedIn:
		return "not signed in"
	case LoginExpired:
		return "login expired"
	}
	return "unknown"
}

// WriteList prints one row per subscription.
func WriteList(w io.Writer, l List) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	host := false
	for _, s := range l.Subscriptions {
		host = host || s.Host != ""
	}
	head := "NAME\tHARNESS\tPLAN\tSTATE\tWEEKLY\tRESETS\tAGE"
	if host {
		head = "HOST\t" + head
	}
	fmt.Fprintln(tw, head)
	for _, s := range l.Subscriptions {
		weekly, resets := "-", "-"
		if m := s.Weekly; m != nil {
			weekly = percentText(m)
			resets = resetText(m, l.GeneratedAt, false)
		}
		age := "-"
		if s.AgeSeconds != nil {
			age = humanDuration(time.Duration(*s.AgeSeconds) * time.Second)
			if s.Stale {
				age += " (stale)"
			}
		}
		row := fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s", s.Name, s.Harness, deref(s.Plan, "-"), StateText(s.State), weekly, resets, age)
		if host {
			row = nonEmpty(s.Host, "-") + "\t" + row
		}
		fmt.Fprintln(tw, row)
	}
	return tw.Flush()
}

// WriteDetail prints everything known about one subscription.
func WriteDetail(w io.Writer, d Detail) error {
	s, now := d.Subscription, d.GeneratedAt
	line := func(k, v string) { fmt.Fprintf(w, "%-10s %s\n", k+":", v) }
	fmt.Fprintln(w, s.Name)
	if s.Host != "" {
		line("host", s.Host)
	}
	line("harness", s.Harness)
	line("plan", withReason(deref(s.Plan, "unknown"), s.Plan == nil, s.PlanReason))
	line("state", withReason(StateText(s.State), s.StateReason != "", s.StateReason))
	if len(s.ConfiguredPlans) > 0 {
		line("spend", "subscriptions.toml plan "+strings.Join(s.ConfiguredPlans, ", "))
	}
	line("source", s.Source)
	if s.ObservedAt != nil {
		read := s.ObservedAt.Format(time.RFC3339)
		if s.AgeSeconds != nil {
			read += " (" + humanDuration(time.Duration(*s.AgeSeconds)*time.Second) + " ago)"
		}
		line("read", read)
	} else {
		line("read", "never ("+nonEmpty(s.ObservedReason, "no reading")+")")
	}
	if s.Stale {
		line("stale", s.StaleReason)
	}
	if len(s.Meters) == 0 {
		fmt.Fprintln(w, "\nno meters")
		return nil
	}
	fmt.Fprintln(w)
	var table bytes.Buffer
	tw := tabwriter.NewWriter(&table, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "METER\tUSED\tRESETS\tNOTE")
	for _, m := range s.Meters {
		note := []string{}
		if m.Detail != "" {
			note = append(note, m.Detail)
		}
		if m.Percent == nil && m.Kind != KindCredits && m.PercentReason != "" {
			note = append(note, m.PercentReason)
		}
		if m.RolledOver {
			note = append(note, m.ResetReason)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", m.Label, percentText(&m), resetText(&m, now, true), strings.Join(note, "; "))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for _, l := range strings.SplitAfter(table.String(), "\n") {
		if l != "" {
			fmt.Fprintln(w, strings.TrimRight(l, " \n"))
		}
	}
	return nil
}

func percentText(m *Meter) string {
	if m.Percent == nil {
		if m.Kind == KindCredits || m.Kind == KindExtraUsage {
			return "-"
		}
		return "?"
	}
	return strconv.FormatFloat(*m.Percent, 'f', -1, 64) + "%"
}

func resetText(m *Meter, now time.Time, long bool) string {
	if m.ResetsAt == nil {
		if m.RolledOver {
			return "rolled over"
		}
		return "-"
	}
	rel := "in " + humanDuration(m.ResetsAt.Sub(now))
	if long {
		return m.ResetsAt.Format(time.RFC3339) + " (" + rel + ")"
	}
	return rel
}

func withReason(v string, show bool, reason string) string {
	if show && reason != "" {
		return v + " (" + reason + ")"
	}
	return v
}

func deref(p *string, fallback string) string {
	if p == nil || *p == "" {
		return fallback
	}
	return *p
}

// humanDuration is a compact duration: 45s, 12m, 3h20m, 2d4h.
func humanDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		h, m := int(d.Hours()), int(d.Minutes())%60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, m)
	}
	days, h := int(d.Hours())/24, int(d.Hours())%24
	if h == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd%dh", days, h)
}
