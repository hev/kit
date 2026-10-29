package trace

import (
	"encoding/json"
	"sort"
)

// Reassemble turns stored chunks back into normalized turns. Parts are
// coordinates, not evidence: a tier omitted during indexing simply has no
// rows and therefore produces no placeholder or gap.
func Reassemble(chunks []Chunk) []Turn {
	type blockKey struct {
		block     int
		blockType string
		toolName  string
	}
	type turnKey struct {
		uuid string
		seq  int64
	}
	type turnParts struct {
		turn   Turn
		blocks map[blockKey][]Chunk
	}
	byTurn := map[turnKey]*turnParts{}
	var order []turnKey
	for _, c := range chunks {
		key := turnKey{uuid: c.TurnUUID, seq: c.Seq}
		tp := byTurn[key]
		if tp == nil {
			tp = &turnParts{
				turn: Turn{SessionID: c.SessionID, TurnUUID: c.TurnUUID, ParentUUID: c.ParentUUID,
					Seq: c.Seq, TS: c.TS, Role: c.Role, Workdir: c.Workdir, Branch: c.Branch,
					Harness: c.Harness, SourcePath: c.SourcePath, IsSidechain: c.IsSidechain},
				blocks: map[blockKey][]Chunk{},
			}
			byTurn[key] = tp
			order = append(order, key)
		}
		bk := blockKey{block: c.Block, blockType: c.BlockType, toolName: c.ToolName}
		tp.blocks[bk] = append(tp.blocks[bk], c)
	}
	sort.SliceStable(order, func(i, j int) bool { return byTurn[order[i]].turn.Seq < byTurn[order[j]].turn.Seq })
	turns := make([]Turn, 0, len(order))
	for _, key := range order {
		tp := byTurn[key]
		keys := make([]blockKey, 0, len(tp.blocks))
		for bk := range tp.blocks {
			keys = append(keys, bk)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].block != keys[j].block {
				return keys[i].block < keys[j].block
			}
			if keys[i].blockType != keys[j].blockType {
				return keys[i].blockType < keys[j].blockType
			}
			return keys[i].toolName < keys[j].toolName
		})
		for _, bk := range keys {
			parts := tp.blocks[bk]
			sort.Slice(parts, func(i, j int) bool { return parts[i].Part < parts[j].Part })
			text := parts[0].Text
			for _, part := range parts[1:] {
				r := []rune(part.Text)
				if len(r) > Overlap {
					r = r[Overlap:]
				} else {
					r = nil
				}
				text += string(r)
			}
			tp.turn.Blocks = append(tp.turn.Blocks, Block{Type: bk.blockType, Text: text, ToolName: bk.toolName})
		}
		restorePiToolResultMetadata(&tp.turn)
		turns = append(turns, tp.turn)
	}
	return CaptureSkills(turns)
}

// PI carries native tool-result status in its searchable event envelope. Restore
// it after chunk reassembly without changing the shared chunk/store schema.
func restorePiToolResultMetadata(t *Turn) {
	if t.Harness != "pi" || t.Role != "tool" {
		return
	}
	for _, b := range t.Blocks {
		if b.Type != "pi_event" {
			continue
		}
		var meta struct {
			ToolCallID string `json:"toolCallId"`
			ToolName   string `json:"toolName"`
			IsError    *bool  `json:"isError"`
		}
		if json.Unmarshal([]byte(b.Text), &meta) != nil || meta.ToolCallID == "" || meta.IsError == nil {
			continue
		}
		for i := range t.Blocks {
			if t.Blocks[i].Type == "tool_result" {
				t.Blocks[i].IsError = *meta.IsError
				t.Blocks[i].ToolUseID = meta.ToolCallID
			}
		}
	}
}
