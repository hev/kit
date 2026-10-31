package dispatch

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Inline builds an order using the same parser as file-based dispatch.
func Inline(o Order, task string, acceptance []string, used map[string]bool, now time.Time) (Order, string, error) {
	if strings.TrimSpace(task) == "" {
		return o, "", fmt.Errorf("task required")
	}
	if o.Name == "" {
		slug := regexp.MustCompile("[^a-z0-9]+").ReplaceAllString(strings.ToLower(task), "-")
		slug = strings.Trim(slug, "-")
		if len(slug) > 48 {
			slug = strings.TrimRight(slug[:48], "-")
		}
		if slug == "" {
			slug = "task"
		}
		base := slug + "-" + now.Format("20060102")
		o.Name = base
		for i := 2; used[o.Name]; i++ {
			o.Name = fmt.Sprintf("%s-%d", base, i)
		}
	}
	if len(acceptance) == 0 {
		acceptance = []string{"The task above is done as described"}
	}
	var b strings.Builder
	b.WriteString("---\n")
	fields := [][2]string{{"name", o.Name}, {"host", o.Host}, {"repo", o.Repo}, {"base", o.Base}, {"harness", o.Harness}, {"model", o.Model}, {"reply", o.Reply}, {"issue", o.Issue}}
	for _, f := range fields {
		if strings.ContainsAny(f[1], "\r\n") {
			return o, "", fmt.Errorf("invalid %s", f[0])
		}
		if f[1] != "" {
			value := f[1]
			quote := "\""
			if strings.Contains(value, quote) {
				quote = "'"
			}
			if strings.Contains(value, quote) {
				return o, "", fmt.Errorf("unsupported quotes in %s", f[0])
			}
			fmt.Fprintf(&b, "%s: %s%s%s\n", f[0], quote, value, quote)
		}
	}
	if o.Timeout != 0 {
		fmt.Fprintf(&b, "timeout: %d\n", o.Timeout)
	}
	b.WriteString("---\n\n" + task + "\n\n## Acceptance\n")
	for _, a := range acceptance {
		b.WriteString("- " + a + "\n")
	}
	parsed, err := Parse(b.String())
	return parsed, b.String(), err
}

// SelectHost prefers configuration, then the reachable host with most free slots.
func SelectHost(c Config, inv map[string]Response) (string, error) {
	if c.DefaultHost != "" {
		return c.DefaultHost, nil
	}
	best := ""
	free := -1
	for _, n := range SortedHosts(inv) {
		r := inv[n]
		if r.Error == "" && c.Hosts[n].Cap-r.Running > free {
			best = n
			free = c.Hosts[n].Cap - r.Running
		}
	}
	if best == "" {
		return "", fmt.Errorf("no reachable hosts")
	}
	return best, nil
}
