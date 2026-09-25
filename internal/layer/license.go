package layer

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// License is the gateway's local license projection (GET /v2/license). The
// gateway verifies its key offline and answers from that, so reading it never
// reaches a license server. Community edition serves the same route with
// reason "open_gateway".
type License struct {
	Valid    bool     `json:"valid"`
	State    string   `json:"state"`
	Reason   string   `json:"reason"`
	Sub      string   `json:"sub"`
	Tier     string   `json:"tier"`
	Features []string `json:"features"`
	Exp      string   `json:"exp"`
	Gateway  struct {
		State                 string `json:"state"`
		SecondsToDeadline     int64  `json:"seconds_to_deadline"`
		GraceSecondsRemaining int64  `json:"grace_seconds_remaining"`
	} `json:"gateway"`
}

// ReasonOpenGateway is the reason community edition gives: the route is there
// and no license can apply.
const ReasonOpenGateway = "open_gateway"

// License reads the gateway's license state. A gateway older than the route
// answers 404, and so did every community gateway before it: both read as
// community.
func (c *Client) License() (License, error) {
	var l License
	req, err := http.NewRequest("GET", c.Endpoint+"/v2/license", nil)
	if err != nil {
		return l, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return l, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		l.State, l.Reason = "floor", ReasonOpenGateway
		return l, nil
	}
	if resp.StatusCode != http.StatusOK {
		return l, fmt.Errorf("GET /v2/license: %s", resp.Status)
	}
	return l, json.NewDecoder(resp.Body).Decode(&l)
}

// Community reports whether the gateway is the community edition.
func (l License) Community() bool { return l.Reason == ReasonOpenGateway }

// Edition is the one line kit prints for the gateway it talks to.
func (l License) Edition() string {
	switch {
	case l.Community():
		return "hev layer community"
	case !l.Valid:
		reason := l.Reason
		if reason == "" {
			reason = l.State
		}
		return "hev layer pro · no valid license (" + reason + ")"
	case l.Gateway.State == "grace":
		return fmt.Sprintf("hev layer pro · %s · expired, grace ends in %s", l.Tier, days(l.Gateway.GraceSecondsRemaining))
	}
	line := "hev layer pro · " + l.Tier
	if t, err := time.Parse(time.RFC3339, l.Exp); err == nil {
		line += " · through " + t.Format("2006-01-02")
	}
	return line
}

// renewWithin is how close to its deadline a license has to be before kit
// says so.
const renewWithin = 14 * 24 * time.Hour

// Warning is a line worth printing on its own, or "": a license that is in
// grace, or that ends within two weeks.
func (l License) Warning() string {
	if !l.Valid {
		return ""
	}
	left := time.Duration(l.Gateway.SecondsToDeadline) * time.Second
	switch {
	case l.Gateway.State == "grace":
		return fmt.Sprintf("the hev layer license has expired; licensed features stop in %s. Renew at https://hevlayer.com/contact", days(l.Gateway.GraceSecondsRemaining))
	case l.Gateway.State == "licensed" && left < renewWithin:
		if l.Tier == "trial" {
			return fmt.Sprintf("the hev layer trial ends in %s; plans are at https://hevlayer.com/pricing", days(l.Gateway.SecondsToDeadline))
		}
		return fmt.Sprintf("the hev layer license expires in %s. Renew at https://hevlayer.com/contact", days(l.Gateway.SecondsToDeadline))
	}
	return ""
}

func days(seconds int64) string {
	d := (seconds + 86399) / 86400
	if d == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", d)
}

// LicenseRequired reports the feature a gateway refused for want of a license:
// the 402 {"error": "license_required", "feature": ...} a floor gateway
// answers on a licensed route.
func LicenseRequired(err error) (feature string, ok bool) {
	var e *HTTPError
	if !errors.As(err, &e) || e.Status != http.StatusPaymentRequired {
		return "", false
	}
	var body struct {
		Error   string `json:"error"`
		Feature string `json:"feature"`
	}
	if json.Unmarshal(e.Body, &body) != nil || body.Error != "license_required" {
		return "", false
	}
	return body.Feature, true
}
