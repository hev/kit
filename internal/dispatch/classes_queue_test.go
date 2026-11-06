package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// classQueueHost is classSourceHost with cap = 1 and one running docs order,
// so every further start is queued.
func classQueueHost(t *testing.T) (start func(name, class string) (Response, error), end func(string)) {
	t.Helper()
	_, cfg := classSourceHost(t)
	b, e := os.ReadFile(cfg)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(cfg, []byte(strings.Replace(string(b), "cap = 10", "cap = 1", 1)), 0600); e != nil {
		t.Fatal(e)
	}
	end = fakeLiveLaunches(t)
	workspace := classWorkspace(t)
	start = func(name, class string) (Response, error) {
		text := classOrder(name, class)
		o, e := Parse(text)
		if e != nil {
			t.Fatal(e)
		}
		return Handle(Request{Action: "start", Name: name, Order: text, Meta: Meta{Order: o}, Workspace: workspace})
	}
	if _, e = start("docs", "docs"); e != nil {
		t.Fatal(e)
	}
	return start, end
}

func queuedNames(t *testing.T) []string {
	t.Helper()
	entries, e := queueEntries()
	if e != nil {
		t.Fatal(e)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Request.Name)
	}
	return names
}

// A start queued on a full host holds no class slot: its probe's reservation
// is released at enqueue and the queue entry keeps no class token, so another
// probe can reserve the class while it waits.
func TestDispatchClassQueuedOrderHoldsNoSlot(t *testing.T) {
	classQueueHost(t)
	text := classOrder("queued", "code")
	probe, e := Handle(Request{Action: "start", Name: "queued", Order: text, AdmissionOnly: true, ReserveClass: true})
	if e != nil || probe.ClassToken == "" {
		t.Fatal(probe, e)
	}
	o, _ := Parse(text)
	r, e := Handle(Request{Action: "start", Name: "queued", Order: text, Meta: Meta{Order: o}, ClassToken: probe.ClassToken})
	if e != nil || !strings.HasPrefix(r.Message, "queued queued") {
		t.Fatalf("expected the full host to queue the start: %+v %v", r, e)
	}
	if exists(classReservationPath("queued")) {
		t.Fatal("queued order kept its probe's class reservation")
	}
	entries, e := queueEntries()
	if e != nil || len(entries) != 1 || entries[0].Request.ClassToken != "" || entries[0].Request.ReserveClass {
		t.Fatalf("queue entry kept class authority: %+v %v", entries, e)
	}
	if n := codeOccupancy(t); n != 0 {
		t.Fatalf("queued order holds a code slot: %d", n)
	}
	other := classOrder("other", "code")
	if r, e = Handle(Request{Action: "start", Name: "other", Order: other, AdmissionOnly: true, ReserveClass: true}); e != nil || !r.ClassReserved {
		t.Fatalf("queued order blocked another code reservation: %+v %v", r, e)
	}
}

// Dequeue admits the class under the root lock like any start. A head whose
// class is full is not launched over the cap: it waits at the head of the
// FIFO, keeps its entry, and starts once the class frees.
func TestDispatchClassDequeueAdmitsClass(t *testing.T) {
	start, end := classQueueHost(t)
	for _, name := range []string{"first", "second"} {
		if r, e := start(name, "code"); e != nil || !strings.HasPrefix(r.Message, "queued "+name) {
			t.Fatalf("%s: %+v %v", name, r, e)
		}
	}
	// A source probe takes the code slot while both wait.
	held, e := Handle(Request{Action: "start", Name: "held", Order: classOrder("held", "code"), AdmissionOnly: true, ReserveClass: true})
	if e != nil || !held.ClassReserved {
		t.Fatal(held, e)
	}
	end("docs")
	if e = SweepQueue(); e != nil {
		t.Fatal(e)
	}
	if exists(filepath.Join(Root(), "first")) {
		t.Fatal("class-full head was launched or recorded as refused instead of waiting")
	}
	if got := strings.Join(queuedNames(t), ","); got != "first,second" {
		t.Fatalf("class-full head left the FIFO: %s", got)
	}
	if n := codeOccupancy(t); n != 1 {
		t.Fatalf("code occupancy %d, want the reservation only", n)
	}
	if e = os.Remove(classReservationPath("held")); e != nil {
		t.Fatal(e)
	}
	if e = SweepQueue(); e != nil {
		t.Fatal(e)
	}
	if !exists(filepath.Join(Root(), "first", "meta.json")) || exists(filepath.Join(Root(), "second")) {
		t.Fatal("dequeue did not start exactly the head once the class freed")
	}
	if n := codeOccupancy(t); n != 1 {
		t.Fatalf("code occupancy %d after dequeue, want 1", n)
	}
}

// A bound sender's start is admitted against the class like any other
// caller, and the sender cannot carry class reservation authority.
func TestDispatchClassBoundSender(t *testing.T) {
	start, end := classHost(t)
	if e := start("alpha", "running", "code"); e != nil {
		t.Fatal(e)
	}
	text := classOrder("mate", "code")
	q := Request{Action: "start", Name: "mate", Order: text, ReserveClass: true, ClassToken: "forged", AdmissionOnly: true}
	if e := authorizeSender(&q, "teammate"); e != nil {
		t.Fatal(e)
	}
	if q.ReserveClass || q.ClassToken != "" || q.AdmissionOnly {
		t.Fatalf("sender kept class authority: %+v", q)
	}
	if _, e := HandleAs(Request{Action: "start", Name: "mate", Order: text, ClassToken: "forged"}, "teammate"); !classFull(e) {
		t.Fatalf("bound sender start bypassed the class cap: %v", e)
	}
	if exists(filepath.Join(Root(), "mate")) {
		t.Fatal("rejected sender start created a run")
	}
	end("running")
	if n := codeOccupancy(t); n != 0 {
		t.Fatalf("code occupancy %d", n)
	}
}
