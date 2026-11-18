// Package collect is `hev collect`: the receiver a host app (an agent built on
// the Vercel AI SDK, for example) exports its OpenTelemetry traces to, and the
// small HTTP API the app's own agent tools call to search those traces and to
// keep memories on a board.
//
// It takes OTLP/HTTP trace exports (JSON or protobuf), scrubs every span with
// kit's redaction policy before anything leaves the process, writes each span
// as a trace envelope, collapses the spans of one turn into a turn document,
// and indexes the turn in a Layer namespace for hybrid search. Nothing here
// tails transcripts, dispatches work or serves a dashboard.
package collect

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"
)

// Span is one OTLP span, flattened to what app capture reads.
type Span struct {
	TraceID       string
	SpanID        string
	ParentSpanID  string
	Name          string
	Start         time.Time
	End           time.Time
	Attrs         map[string]any
	StatusCode    int // 0 unset, 1 ok, 2 error (OTLP StatusCode)
	StatusMessage string
	Resource      map[string]any
	Scope         string
	// Redactions is how many replacements Redact made in this span.
	Redactions int
}

// StatusError is OTLP's STATUS_CODE_ERROR.
const StatusError = 2

// ---- OTLP/JSON -----------------------------------------------------------

type jsonRequest struct {
	ResourceSpans []struct {
		Resource struct {
			Attributes []jsonKV `json:"attributes"`
		} `json:"resource"`
		ScopeSpans []struct {
			Scope struct {
				Name string `json:"name"`
			} `json:"scope"`
			Spans []jsonSpan `json:"spans"`
		} `json:"scopeSpans"`
	} `json:"resourceSpans"`
}

type jsonSpan struct {
	TraceID      string          `json:"traceId"`
	SpanID       string          `json:"spanId"`
	ParentSpanID string          `json:"parentSpanId"`
	Name         string          `json:"name"`
	Start        json.RawMessage `json:"startTimeUnixNano"`
	End          json.RawMessage `json:"endTimeUnixNano"`
	Attributes   []jsonKV        `json:"attributes"`
	Status       struct {
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	} `json:"status"`
}

type jsonKV struct {
	Key   string    `json:"key"`
	Value jsonValue `json:"value"`
}

type jsonValue struct {
	StringValue *string         `json:"stringValue"`
	BoolValue   *bool           `json:"boolValue"`
	IntValue    json.RawMessage `json:"intValue"`
	DoubleValue *float64        `json:"doubleValue"`
	BytesValue  *string         `json:"bytesValue"`
	ArrayValue  *struct {
		Values []jsonValue `json:"values"`
	} `json:"arrayValue"`
	KvlistValue *struct {
		Values []jsonKV `json:"values"`
	} `json:"kvlistValue"`
}

func (v jsonValue) value() any {
	switch {
	case v.StringValue != nil:
		return *v.StringValue
	case v.BoolValue != nil:
		return *v.BoolValue
	case len(v.IntValue) > 0:
		n, _ := jsonInt(v.IntValue)
		return n
	case v.DoubleValue != nil:
		return *v.DoubleValue
	case v.BytesValue != nil:
		return *v.BytesValue
	case v.ArrayValue != nil:
		out := make([]any, 0, len(v.ArrayValue.Values))
		for _, e := range v.ArrayValue.Values {
			out = append(out, e.value())
		}
		return out
	case v.KvlistValue != nil:
		return kvMap(v.KvlistValue.Values)
	}
	return nil
}

func kvMap(kvs []jsonKV) map[string]any {
	m := make(map[string]any, len(kvs))
	for _, kv := range kvs {
		m[kv.Key] = kv.Value.value()
	}
	return m
}

// jsonInt reads a 64-bit integer that OTLP/JSON writes either as a string
// (the spec's form for int64 and fixed64) or as a bare number.
func jsonInt(raw json.RawMessage) (int64, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if u, err := strconv.ParseUint(s, 10, 64); err == nil {
			return int64(u), nil
		}
		return strconv.ParseInt(s, 10, 64)
	}
	var f json.Number
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0, err
	}
	if n, err := f.Int64(); err == nil {
		return n, nil
	}
	ff, err := f.Float64()
	if err != nil {
		return 0, err
	}
	return int64(ff), nil
}

func nanos(raw json.RawMessage) time.Time {
	if len(raw) == 0 {
		return time.Time{}
	}
	n, err := jsonInt(raw)
	if err != nil || n <= 0 {
		return time.Time{}
	}
	return time.Unix(0, n).UTC()
}

// jsonID reads a trace or span id. The OTLP/JSON spec writes ids as hex; some
// exporters write the protobuf JSON mapping's base64 instead.
func jsonID(s string, size int) string {
	if s == "" {
		return ""
	}
	if len(s) == size*2 {
		if _, err := hex.DecodeString(s); err == nil {
			return s
		}
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil && len(b) == size {
		return hex.EncodeToString(b)
	}
	return s
}

// DecodeJSON reads an OTLP/JSON ExportTraceServiceRequest.
func DecodeJSON(body []byte) ([]Span, error) {
	var req jsonRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("otlp json: %w", err)
	}
	var out []Span
	for _, rs := range req.ResourceSpans {
		resource := kvMap(rs.Resource.Attributes)
		for _, ss := range rs.ScopeSpans {
			for _, js := range ss.Spans {
				code := 0
				if len(js.Status.Code) > 0 {
					if n, err := jsonInt(js.Status.Code); err == nil {
						code = int(n)
					} else {
						var name string
						if json.Unmarshal(js.Status.Code, &name) == nil && name == "STATUS_CODE_ERROR" {
							code = StatusError
						}
					}
				}
				out = append(out, Span{
					TraceID: jsonID(js.TraceID, 16), SpanID: jsonID(js.SpanID, 8), ParentSpanID: jsonID(js.ParentSpanID, 8),
					Name: js.Name, Start: nanos(js.Start), End: nanos(js.End),
					Attrs: kvMap(js.Attributes), StatusCode: code, StatusMessage: js.Status.Message,
					Resource: resource, Scope: ss.Scope.Name,
				})
			}
		}
	}
	return out, nil
}

// ---- OTLP/protobuf ---------------------------------------------------------
//
// A hand-rolled reader for the handful of fields app capture uses, so kit
// takes no protobuf dependency. Field numbers are opentelemetry-proto's
// (collector/trace/v1, trace/v1, common/v1, resource/v1). Unknown fields are
// skipped by wire type.

var errProto = errors.New("otlp protobuf: malformed message")

type pbReader struct {
	b []byte
}

func (r *pbReader) varint() (uint64, error) {
	var v uint64
	for shift := uint(0); shift < 64; shift += 7 {
		if len(r.b) == 0 {
			return 0, errProto
		}
		c := r.b[0]
		r.b = r.b[1:]
		v |= uint64(c&0x7f) << shift
		if c < 0x80 {
			return v, nil
		}
	}
	return 0, errProto
}

func (r *pbReader) fixed64() (uint64, error) {
	if len(r.b) < 8 {
		return 0, errProto
	}
	var v uint64
	for i := 7; i >= 0; i-- {
		v = v<<8 | uint64(r.b[i])
	}
	r.b = r.b[8:]
	return v, nil
}

func (r *pbReader) bytes() ([]byte, error) {
	n, err := r.varint()
	if err != nil {
		return nil, err
	}
	if uint64(len(r.b)) < n {
		return nil, errProto
	}
	out := r.b[:n]
	r.b = r.b[n:]
	return out, nil
}

// next reads one field. For wire type 2 it returns the bytes; for 0 and 1 the
// integer; type 5 (fixed32) is read into the integer too.
func (r *pbReader) next() (field int, wire int, n uint64, b []byte, err error) {
	key, err := r.varint()
	if err != nil {
		return 0, 0, 0, nil, err
	}
	field, wire = int(key>>3), int(key&7)
	switch wire {
	case 0:
		n, err = r.varint()
	case 1:
		n, err = r.fixed64()
	case 2:
		b, err = r.bytes()
	case 5:
		if len(r.b) < 4 {
			return 0, 0, 0, nil, errProto
		}
		n = uint64(r.b[0]) | uint64(r.b[1])<<8 | uint64(r.b[2])<<16 | uint64(r.b[3])<<24
		r.b = r.b[4:]
	default:
		err = errProto
	}
	return
}

func pbEach(b []byte, fn func(field, wire int, n uint64, b []byte) error) error {
	r := &pbReader{b: b}
	for len(r.b) > 0 {
		field, wire, n, sub, err := r.next()
		if err != nil {
			return err
		}
		if err := fn(field, wire, n, sub); err != nil {
			return err
		}
	}
	return nil
}

func pbAnyValue(b []byte) (any, error) {
	var out any
	err := pbEach(b, func(field, wire int, n uint64, sub []byte) error {
		switch field {
		case 1:
			out = string(sub)
		case 2:
			out = n != 0
		case 3:
			out = int64(n)
		case 4:
			out = math.Float64frombits(n)
		case 5:
			arr := []any{}
			err := pbEach(sub, func(f, _ int, _ uint64, v []byte) error {
				if f == 1 {
					val, err := pbAnyValue(v)
					if err != nil {
						return err
					}
					arr = append(arr, val)
				}
				return nil
			})
			if err != nil {
				return err
			}
			out = arr
		case 6:
			m := map[string]any{}
			err := pbEach(sub, func(f, _ int, _ uint64, v []byte) error {
				if f == 1 {
					return pbKV(v, m)
				}
				return nil
			})
			if err != nil {
				return err
			}
			out = m
		case 7:
			out = base64.StdEncoding.EncodeToString(sub)
		}
		return nil
	})
	return out, err
}

func pbKV(b []byte, into map[string]any) error {
	var key string
	var val any
	err := pbEach(b, func(field, _ int, _ uint64, sub []byte) error {
		switch field {
		case 1:
			key = string(sub)
		case 2:
			v, err := pbAnyValue(sub)
			if err != nil {
				return err
			}
			val = v
		}
		return nil
	})
	if err == nil && key != "" {
		into[key] = val
	}
	return err
}

func pbSpan(b []byte, resource map[string]any, scope string) (Span, error) {
	s := Span{Attrs: map[string]any{}, Resource: resource, Scope: scope}
	err := pbEach(b, func(field, _ int, n uint64, sub []byte) error {
		switch field {
		case 1:
			s.TraceID = hex.EncodeToString(sub)
		case 2:
			s.SpanID = hex.EncodeToString(sub)
		case 4:
			s.ParentSpanID = hex.EncodeToString(sub)
		case 5:
			s.Name = string(sub)
		case 7:
			if n > 0 {
				s.Start = time.Unix(0, int64(n)).UTC()
			}
		case 8:
			if n > 0 {
				s.End = time.Unix(0, int64(n)).UTC()
			}
		case 9:
			return pbKV(sub, s.Attrs)
		case 15:
			return pbEach(sub, func(f, _ int, n uint64, v []byte) error {
				switch f {
				case 2:
					s.StatusMessage = string(v)
				case 3:
					s.StatusCode = int(n)
				}
				return nil
			})
		}
		return nil
	})
	return s, err
}

// DecodeProto reads an OTLP/protobuf ExportTraceServiceRequest.
func DecodeProto(body []byte) ([]Span, error) {
	var out []Span
	err := pbEach(body, func(field, _ int, _ uint64, rs []byte) error {
		if field != 1 {
			return nil
		}
		resource := map[string]any{}
		var scopes [][]byte
		if err := pbEach(rs, func(f, _ int, _ uint64, v []byte) error {
			switch f {
			case 1:
				return pbEach(v, func(rf, _ int, _ uint64, kv []byte) error {
					if rf == 1 {
						return pbKV(kv, resource)
					}
					return nil
				})
			case 2:
				scopes = append(scopes, v)
			}
			return nil
		}); err != nil {
			return err
		}
		for _, ss := range scopes {
			var scope string
			var spans [][]byte
			if err := pbEach(ss, func(f, _ int, _ uint64, v []byte) error {
				switch f {
				case 1:
					return pbEach(v, func(sf, _ int, _ uint64, name []byte) error {
						if sf == 1 {
							scope = string(name)
						}
						return nil
					})
				case 2:
					spans = append(spans, v)
				}
				return nil
			}); err != nil {
				return err
			}
			for _, sp := range spans {
				s, err := pbSpan(sp, resource, scope)
				if err != nil {
					return err
				}
				out = append(out, s)
			}
		}
		return nil
	})
	return out, err
}
