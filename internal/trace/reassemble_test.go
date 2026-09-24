package trace

import (
	"strings"
	"testing"
)

func TestReassembleTrimsExactRuneOverlap(t *testing.T) {
	want := strings.Repeat("🙂", MaxRunes+37)
	turn := Turn{SessionID: "s", TurnUUID: "u", Seq: 2, Role: "assistant", Blocks: []Block{{Type: "text", Text: want}}}
	got := Reassemble(Chunks(turn))
	if len(got) != 1 || len(got[0].Blocks) != 1 {
		t.Fatalf("Reassemble = %+v", got)
	}
	if got[0].Blocks[0].Text != want {
		t.Errorf("seam was not restored exactly: got %d runes, want %d", len([]rune(got[0].Blocks[0].Text)), len([]rune(want)))
	}
}

func TestReassemblePreservesBlockOrderAndIgnoresAbsentTiers(t *testing.T) {
	turn := Turn{SessionID: "s", TurnUUID: "u", Blocks: []Block{
		{Type: "text", Text: "before"},
		{Type: "tool_result", Text: "not indexed"},
		{Type: "text", Text: "after"},
	}}
	chunks := Chunks(turn)
	chunks = []Chunk{chunks[2], chunks[0]} // model a tier-filtered, unordered fetch
	got := Reassemble(chunks)
	if len(got[0].Blocks) != 2 || got[0].Blocks[0].Text != "before" || got[0].Blocks[1].Text != "after" {
		t.Fatalf("blocks = %+v", got[0].Blocks)
	}
}
