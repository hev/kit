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

// A lower ceiling, for a model with a small input limit, keeps every chunk
// under it and still reassembles exactly: the overlap does not change.
func TestChunksMaxStaysUnderTheCeilingAndReassembles(t *testing.T) {
	want := strings.Repeat("é", 3*MaxRunes+11)
	turn := Turn{SessionID: "s", TurnUUID: "u", Role: "assistant", Blocks: []Block{{Type: "text", Text: want}}}
	chunks := ChunksMax(turn, 510)
	if len(chunks) < 8 {
		t.Fatalf("%d chunks", len(chunks))
	}
	for i, c := range chunks {
		if n := len([]rune(c.Text)); n > 510 {
			t.Fatalf("chunk %d has %d runes", i, n)
		}
	}
	if got := Reassemble(chunks); got[0].Blocks[0].Text != want {
		t.Fatal("seam was not restored exactly")
	}
	for _, bad := range []int{0, Overlap, MaxRunes + 1} {
		if got, full := ChunksMax(turn, bad), Chunks(turn); len(got) != len(full) {
			t.Fatalf("ceiling %d: %d chunks, want the default %d", bad, len(got), len(full))
		}
	}
}
