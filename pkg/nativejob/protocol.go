package nativejob

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// Call is a local-owner protocol, not a remote authentication mechanism. A
// deployment must authenticate enrollment/authority before exposing a gateway.
// Unknown JSON fields and trailing data fail closed.
type Call struct {
	Version      string      `json:"version"`
	Action       string      `json:"action"`
	Request      *Request    `json:"request,omitempty"`
	Fence        *Fence      `json:"fence,omitempty"`
	JobID        string      `json:"job_id,omitempty"`
	Identity     *Identity   `json:"identity,omitempty"`
	EventID      string      `json:"event_id,omitempty"`
	ResultDigest string      `json:"result_digest,omitempty"`
	Inspection   *Inspection `json:"inspection,omitempty"`
}

func DecodeCall(r io.Reader) (Call, error) {
	var c Call
	data, err := io.ReadAll(io.LimitReader(r, (1<<20)+1))
	if err != nil {
		return c, err
	}
	if len(data) > 1<<20 {
		return c, errors.New("call exceeds 1 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return c, errors.New("trailing data")
	}
	return c, nil
}
func (s *Store) Call(c Call) (any, error) {
	if c.Version != Version {
		return nil, errors.New("unsupported protocol version")
	}
	// Reject unused fields so ambiguous envelopes cannot silently mutate state.
	shape := Call{Version: c.Version, Action: c.Action}
	switch c.Action {
	case "submit":
		shape.Request = c.Request
	case "fence":
		shape.Fence = c.Fence
	case "query", "execute", "recover":
		shape.JobID = c.JobID
	case "outbox":
	case "ack":
		shape.JobID = c.JobID
		shape.Identity = c.Identity
		shape.EventID = c.EventID
		shape.ResultDigest = c.ResultDigest
	case "reconcile":
		shape.JobID = c.JobID
		shape.Identity = c.Identity
		shape.Inspection = c.Inspection
	default:
		return nil, errors.New("unsupported action")
	}
	if !bytes.Equal([]byte(encode(shape)), []byte(encode(c))) {
		return nil, errors.New("unexpected action fields")
	}
	switch c.Action {
	case "submit":
		if c.Request != nil {
			return s.Submit(*c.Request)
		}
	case "fence":
		if c.Fence != nil {
			return nil, s.AdvanceFence(*c.Fence)
		}
	case "query":
		if c.JobID != "" {
			return s.Query(c.JobID)
		}
	case "execute":
		if c.JobID != "" {
			return s.Execute(c.JobID)
		}
	case "recover":
		if c.JobID != "" {
			return s.Recover(c.JobID)
		}
	case "outbox":
		return s.Outbox()
	case "ack":
		if c.Identity != nil && c.JobID != "" && c.EventID != "" && c.ResultDigest != "" {
			return nil, s.Ack(c.JobID, *c.Identity, c.EventID, c.ResultDigest)
		}
	case "reconcile":
		if c.Identity != nil && c.Inspection != nil && c.JobID != "" {
			return s.Reconcile(c.JobID, *c.Identity, *c.Inspection)
		}
	}
	return nil, errors.New("missing action fields")
}
