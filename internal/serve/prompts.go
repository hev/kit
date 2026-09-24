package serve

import (
	"fmt"
	"net/url"
	"strconv"
	"time"
	_ "time/tzdata"

	"github.com/hev/kit/internal/trace"
)

// Both heatmap aggregation and selection use the browser's IANA timezone,
// preserving daylight-saving transitions. Direct API callers default to UTC.
func promptLocation(q url.Values) (*time.Location, error) {
	zone := q.Get("timezone")
	if zone == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return nil, fmt.Errorf("unknown timezone %q", zone)
	}
	return loc, nil
}

func promptRows(rows []trace.SessionRow, q url.Values) ([]trace.SessionRow, error) {
	loc, err := promptLocation(q)
	if err != nil {
		return nil, err
	}
	if !q.Has("weekday") && !q.Has("hour") {
		return rows, nil
	}
	weekday, dayErr := strconv.Atoi(q.Get("weekday"))
	hour, hourErr := strconv.Atoi(q.Get("hour"))
	if dayErr != nil || hourErr != nil || weekday < 0 || weekday > 6 || hour < 0 || hour > 23 {
		return nil, fmt.Errorf("weekday (0–6) and hour (0–23) must be supplied together")
	}
	out := []trace.SessionRow{}
	for _, row := range rows {
		for _, ts := range row.PromptTS {
			d := time.UnixMilli(int64(ts)).In(loc)
			if int(d.Weekday()) == weekday && d.Hour() == hour {
				out = append(out, row)
				break
			}
		}
	}
	return out, nil
}
