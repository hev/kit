package nativehost

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestArgoTaskCodecSeparateAndStrict(t *testing.T) {
	task := []byte(`{"version":"factory-argo-task/v1","order":"o"}`)
	raw, err := EncodeArgoTaskEnvelope("profile", hash([]byte("config")), task)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeArgoTaskEnvelope(raw)
	if err != nil || !bytes.Equal(got.TaskReference, task) {
		t.Fatal(got, err)
	}
	// Routing JSON is never accepted by the existing receiver protocol.
	if receiverCall(raw) == nil || receiverCall(task) == nil {
		t.Fatal("mixed protocols")
	}
	for _, bad := range []string{
		strings.Replace(string(raw), `"profile_id":"profile"`, `"profile_id":"profile","PROFILE_ID":"other"`, 1),
		strings.Replace(string(raw), `"order":"o"`, `"order":"o","order":"other"`, 1),
		strings.Replace(string(raw), `"version":"native-argo-task-rpc/v1"`, `"version":"native-receiver-rpc/v1"`, 1),
		strings.TrimSuffix(string(raw), "}") + `,"argv":[]}`,
		strings.Replace(string(raw), string(task), `null`, 1),
		strings.Replace(string(raw), string(task), `[]`, 1),
	} {
		if _, err := DecodeArgoTaskEnvelope([]byte(bad)); err == nil {
			t.Fatal(bad)
		}
	}
	if _, err := EncodeArgoTaskEnvelope("p", hash([]byte("c")), json.RawMessage(`{"x":"`+strings.Repeat("x", ArgoTaskReferenceLimit)+`"}`)); err == nil {
		t.Fatal("size limit")
	}
}
