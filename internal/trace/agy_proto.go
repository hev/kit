package trace

import (
	"encoding/binary"
	"errors"
	"net/url"
	"strings"
	"time"
)

// agy (Google's Antigravity CLI) stores each conversation step as a protobuf
// message with no published schema. The field numbers below were read from
// the descriptors compiled into agy 1.5.12 (gemini_coder.Step in
// third_party/gemini_coder/proto/trajectory.proto, its exa.cortex_pb and
// exa.codeium_common_pb submessages), so each constant carries the name agy
// gives it. Decoding is by hand over the wire format: a field this file does
// not name is skipped, whatever its type, so a newer agy that adds fields or
// step types still reads.

// gemini_coder.Step
const (
	agyStepType          = 1   // exa.cortex_pb.CortexStepType
	agyStepStatus        = 4   // exa.cortex_pb.CortexStepStatus
	agyStepMetadata      = 5   // CortexStepMetadata
	agyStepUserInput     = 19  // CortexStepUserInput
	agyStepPlanner       = 20  // CortexStepPlannerResponse
	agyStepErrorMessage  = 24  // CortexStepErrorMessage
	agyStepError         = 31  // CortexErrorDetails
	agyStepSystemMessage = 114 // CortexStepSystemMessage
	agyStepGeneric       = 140 // CortexStepGeneric
)

// exa.cortex_pb.CortexStepType values agy 1.5.12 writes for a conversation.
// Every tool agy 1.5.12 ships (run_command, read_file, write_file, edit_file,
// search_web, ...) records its result as a GENERIC step.
const (
	agyTypeUserInput       = 14  // CORTEX_STEP_TYPE_USER_INPUT
	agyTypePlannerResponse = 15  // CORTEX_STEP_TYPE_PLANNER_RESPONSE
	agyTypeErrorMessage    = 17  // CORTEX_STEP_TYPE_ERROR_MESSAGE
	agyTypeSystemMessage   = 101 // CORTEX_STEP_TYPE_SYSTEM_MESSAGE
	agyTypeGeneric         = 132 // CORTEX_STEP_TYPE_GENERIC
)

// exa.cortex_pb.CortexStepStatus
const (
	agyStatusPending     = 1
	agyStatusRunning     = 2
	agyStatusDone        = 3
	agyStatusCanceled    = 6
	agyStatusError       = 7
	agyStatusGenerating  = 8
	agyStatusWaiting     = 9
	agyStatusQueued      = 11
	agyStatusInterrupted = 12
)

// agyUnfinished reports a step agy has not finished writing: its text or
// result may still change.
func agyUnfinished(status uint64) bool {
	switch status {
	case agyStatusPending, agyStatusRunning, agyStatusGenerating, agyStatusWaiting, agyStatusQueued:
		return true
	}
	return false
}

// pbField is one field of a protobuf message on the wire.
type pbField struct {
	num  uint64
	wire uint8
	v    uint64 // varint, fixed32 or fixed64 value
	b    []byte // length-delimited payload
}

// pbMsg is a parsed message: its fields in wire order, repeats kept.
type pbMsg []pbField

var errPBMalformed = errors.New("malformed protobuf")

func pbUvarint(b []byte) (uint64, int) {
	v, n := binary.Uvarint(b)
	if n <= 0 {
		return 0, 0
	}
	return v, n
}

// pbParse reads one message level. Groups (wire types 3 and 4, unused by
// proto3) and truncated input are errors, never panics.
func pbParse(b []byte) (pbMsg, error) {
	var m pbMsg
	for len(b) > 0 {
		key, n := pbUvarint(b)
		if n == 0 {
			return nil, errPBMalformed
		}
		b = b[n:]
		f := pbField{num: key >> 3, wire: uint8(key & 7)}
		if f.num == 0 {
			return nil, errPBMalformed
		}
		switch f.wire {
		case 0:
			v, n := pbUvarint(b)
			if n == 0 {
				return nil, errPBMalformed
			}
			f.v, b = v, b[n:]
		case 1:
			if len(b) < 8 {
				return nil, errPBMalformed
			}
			f.v, b = binary.LittleEndian.Uint64(b), b[8:]
		case 2:
			l, n := pbUvarint(b)
			if n == 0 || l > uint64(len(b)-n) {
				return nil, errPBMalformed
			}
			f.b, b = b[n:n+int(l)], b[n+int(l):]
		case 5:
			if len(b) < 4 {
				return nil, errPBMalformed
			}
			f.v, b = uint64(binary.LittleEndian.Uint32(b)), b[4:]
		default:
			return nil, errPBMalformed
		}
		m = append(m, f)
	}
	return m, nil
}

// uint returns the last varint value of field num (proto3: last one wins).
func (m pbMsg) uint(num uint64) uint64 {
	var v uint64
	for _, f := range m {
		if f.num == num && f.wire == 0 {
			v = f.v
		}
	}
	return v
}

func (m pbMsg) has(num uint64) bool {
	for _, f := range m {
		if f.num == num {
			return true
		}
	}
	return false
}

// str returns the last length-delimited value of field num as a string.
func (m pbMsg) str(num uint64) string {
	var s string
	for _, f := range m {
		if f.num == num && f.wire == 2 {
			s = string(f.b)
		}
	}
	return s
}

func (m pbMsg) bytes(num uint64) []byte {
	var b []byte
	for _, f := range m {
		if f.num == num && f.wire == 2 {
			b = f.b
		}
	}
	return b
}

// strs returns every value of a repeated string field.
func (m pbMsg) strs(num uint64) []string {
	var out []string
	for _, f := range m {
		if f.num == num && f.wire == 2 {
			out = append(out, string(f.b))
		}
	}
	return out
}

// msg returns submessage num, empty when absent or malformed.
func (m pbMsg) msg(num uint64) pbMsg {
	var out pbMsg
	for _, f := range m {
		if f.num == num && f.wire == 2 {
			if sub, err := pbParse(f.b); err == nil {
				out = sub
			}
		}
	}
	return out
}

// msgs returns every well-formed value of a repeated submessage field.
func (m pbMsg) msgs(num uint64) []pbMsg {
	var out []pbMsg
	for _, f := range m {
		if f.num == num && f.wire == 2 {
			if sub, err := pbParse(f.b); err == nil {
				out = append(out, sub)
			}
		}
	}
	return out
}

// uints returns a repeated integer field, packed or not.
func (m pbMsg) uints(num uint64) []uint64 {
	var out []uint64
	for _, f := range m {
		if f.num != num {
			continue
		}
		switch f.wire {
		case 0:
			out = append(out, f.v)
		case 2:
			for b := f.b; len(b) > 0; {
				v, n := pbUvarint(b)
				if n == 0 {
					break
				}
				out = append(out, v)
				b = b[n:]
			}
		}
	}
	return out
}

// sint32 reads a varint field declared int32: negative values are written as
// ten-byte two's complement.
func (m pbMsg) sint32(num uint64) int64 { return int64(int32(m.uint(num))) }

// pbTime formats a google.protobuf.Timestamp (1 seconds, 2 nanos).
func pbTime(ts pbMsg) string {
	sec, nanos := int64(ts.uint(1)), int64(ts.uint(2))
	if sec <= 0 || nanos < 0 || nanos >= 1e9 {
		return ""
	}
	return time.Unix(sec, nanos).UTC().Format(time.RFC3339Nano)
}

// agyFileURI turns agy's workspace URIs (file:///home/..., file:///C:/...)
// into a native path; anything else is returned as it is.
func agyFileURI(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return uri
	}
	p := u.Path
	if u.Host != "" && u.Host != "localhost" {
		return `\\` + u.Host + strings.ReplaceAll(p, "/", `\`) // UNC share
	}
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		return strings.ReplaceAll(p[1:], "/", `\`) // /C:/Users -> C:\Users
	}
	return p
}
