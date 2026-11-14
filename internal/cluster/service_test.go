package cluster

import "testing"

func TestFactsRejectionsCountedWithoutLogging(t *testing.T) {
	s := &Service{}
	if err := s.PublishNodeFacts(newCounters("alpha", true), NodeFacts{}); err == nil {
		t.Fatal("invalid facts accepted")
	}
	if err := s.PublishNodeFacts(newCounters("alpha", true), NodeFacts{}); err == nil {
		t.Fatal("invalid facts accepted")
	}
	if got := s.Status().FactsRejected; got != 2 {
		t.Fatal(got)
	}
}
