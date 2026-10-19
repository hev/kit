package nativegate

import (
	"encoding/json"
	"github.com/hev/factory/pkg/argohost"
	"strings"
	"testing"
)

func TestArgoReferenceFacadeAndBound(t *testing.T) {
	r := argohost.TaskReference{Version: argohost.TaskReferenceVersion, Order: "order", SourceDigest: "sha256:" + strings.Repeat("a", 64), Epoch: "e1", Step: "implement", Attempt: "a1", WorkflowUID: "workflow", NodeID: "node", ParentArtifacts: []argohost.ParentArtifactRef{}}
	b, _ := json.Marshal(r)
	got, err := DecodeArgoTaskReference(strings.NewReader(string(b)))
	gd, ge := got.BindingDigest()
	rd, re := r.BindingDigest()
	if err != nil || ge != nil || re != nil || got.Key() != r.Key() || gd != rd {
		t.Fatal(got, err)
	}
	for _, bad := range []string{string(b) + "{}", strings.TrimSuffix(string(b), "}") + `,"argv":[]}`, strings.Replace(string(b), `"order":"order"`, `"order":"order","order":"other"`, 1), strings.Repeat("x", argohost.TaskReferenceLimit+1), `null`} {
		if _, err := DecodeArgoTaskReference(strings.NewReader(bad)); err == nil {
			t.Fatal(bad)
		}
	}
}
