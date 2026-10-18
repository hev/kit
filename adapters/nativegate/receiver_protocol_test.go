package nativegate

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReceiverProtocolRejectsAmbiguityAndExecutionPayload(t *testing.T) {
	good := `{"version":"native-receiver/v1","operation":"inspect","job_id":"job-original"}`
	for _, raw := range []string{good, good + "\n"} {
		if _, err := DecodeReceiverCall(strings.NewReader(raw)); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{
		strings.Replace(good, `"version"`, `"VERSION"`, 1),
		strings.Replace(good, `"job_id":"job-original"`, `"job_id":"job-original","job_id":"other"`, 1),
		strings.Replace(good, `"job_id":"job-original"`, `"job_id":"job-original","claim":null`, 1),
		strings.Replace(good, `"job_id":"job-original"`, `"job_id":"job-original","argv":["/bin/sh"]`, 1),
		good + ` {}`, good + strings.Repeat(" ", 64<<10),
		`{"version":"native-receiver/v1","operation":"start","job_id":"job-original","claim":null,"binding":null}`,
		`{"version":"native-receiver/v1","operation":"ack","job_id":"job-original","identity":{},"event_id":"","result_digest":""}`,
	} {
		if _, err := DecodeReceiverCall(strings.NewReader(raw)); err == nil {
			t.Fatal("ambiguous input accepted", raw[:min(len(raw), 160)])
		}
	}
	c, _, _ := sample()
	claim := dbClaim()
	start := ReceiverCall{Version: ReceiverVersion, Operation: "start", JobID: c.JobID, Claim: &claim}
	// A missing native binding cannot turn a claim into a detached permit.
	raw, _ := json.Marshal(start)
	if _, err := DecodeReceiverCall(strings.NewReader(string(raw))); err == nil {
		t.Fatal("missing binding accepted")
	}
	ack := ReceiverCall{Version: ReceiverVersion, Operation: "ack", JobID: c.JobID, Identity: &c.Request.Identity, EventID: "event", ResultDigest: "digest"}
	raw, _ = json.Marshal(ack)
	if _, err := DecodeReceiverCall(strings.NewReader(string(raw))); err != nil {
		t.Fatal(err)
	}
	alias := strings.Replace(string(raw), `"chef_id"`, `"Chef_ID"`, 1)
	if _, err := DecodeReceiverCall(strings.NewReader(alias)); err == nil {
		t.Fatal("nested alias accepted")
	}
	duplicate := strings.Replace(string(raw), `"chef_id":"chef"`, `"chef_id":"chef","chef_id":"other"`, 1)
	if _, err := DecodeReceiverCall(strings.NewReader(duplicate)); err == nil {
		t.Fatal("nested duplicate accepted")
	}
}
