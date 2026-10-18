package nativegate

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"

	a "github.com/hev/factory/pkg/nativeadmission"
	n "github.com/hev/kit/pkg/nativejob"
)

var ErrReceiverCall = errors.New("invalid native receiver call")

// ReceiverCall carries references and exact immutable claim/result tuples only.
// Executable arguments, environment, issuance and enrollment cannot be submitted.
type ReceiverCall struct {
	Version      string           `json:"version"`
	Operation    string           `json:"operation"`
	JobID        string           `json:"job_id"`
	Claim        *a.DBClaim       `json:"claim,omitempty"`
	Binding      *a.NativeBinding `json:"binding,omitempty"`
	Identity     *n.Identity      `json:"identity,omitempty"`
	EventID      string           `json:"event_id,omitempty"`
	ResultDigest string           `json:"result_digest,omitempty"`
}

func DecodeReceiverCall(r io.Reader) (ReceiverCall, error) {
	var c ReceiverCall
	data, err := decodeExact(r, &c)
	if err != nil {
		return c, ErrReceiverCall
	}
	if c.Version != ReceiverVersion || c.JobID == "" {
		return c, ErrReceiverCall
	}
	allowed := map[string]bool{"version": true, "operation": true, "job_id": true}
	switch c.Operation {
	case "inspect":
	case "start":
		allowed["claim"] = true
		allowed["binding"] = true
		if c.Claim == nil || c.Binding == nil || c.Binding.JobID != c.JobID {
			return c, ErrReceiverCall
		}
	case "ack":
		allowed["identity"] = true
		allowed["event_id"] = true
		allowed["result_digest"] = true
		if c.Identity == nil || c.EventID == "" || c.ResultDigest == "" {
			return c, ErrReceiverCall
		}
	default:
		return c, ErrReceiverCall
	}
	var members map[string]json.RawMessage
	if json.Unmarshal(data, &members) != nil {
		return c, ErrReceiverCall
	}
	for key := range members {
		if !allowed[key] {
			return c, ErrReceiverCall
		}
	}
	for key := range allowed {
		if _, ok := members[key]; !ok {
			return c, ErrReceiverCall
		}
	}
	return c, nil
}

// decodeExact rejects duplicate and case-aliased fields recursively, trailing
// values and oversized calls/configs. Errors do not contain untrusted input.
func decodeExact(r io.Reader, out any) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, (64<<10)+1))
	if err != nil || len(data) > 64<<10 {
		return nil, ErrReceiverCall
	}
	shape := json.NewDecoder(bytes.NewReader(data))
	shape.UseNumber()
	if err = exactShape(shape, reflect.TypeOf(out).Elem()); err != nil {
		return nil, ErrReceiverCall
	}
	if _, err = shape.Token(); err != io.EOF {
		return nil, ErrReceiverCall
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(out); err != nil {
		return nil, ErrReceiverCall
	}
	return data, nil
}
func exactShape(d *json.Decoder, typ reflect.Type) error {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	tok, err := d.Token()
	if err != nil {
		return err
	}
	if tok == nil {
		return nil
	}
	if typ.PkgPath() == "time" && typ.Name() == "Time" {
		if _, ok := tok.(string); !ok {
			return ErrReceiverCall
		}
		return nil
	}
	switch typ.Kind() {
	case reflect.Struct:
		if tok != json.Delim('{') {
			return ErrReceiverCall
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if !f.IsExported() {
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			fields[name] = f.Type
		}
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return ErrReceiverCall
			}
			field, ok := fields[name]
			if !ok || seen[name] {
				return ErrReceiverCall
			}
			seen[name] = true
			if err = exactShape(d, field); err != nil {
				return err
			}
		}
		_, err = d.Token()
		return err
	case reflect.Slice:
		if tok != json.Delim('[') {
			return ErrReceiverCall
		}
		for d.More() {
			if err = exactShape(d, typ.Elem()); err != nil {
				return err
			}
		}
		_, err = d.Token()
		return err
	default:
		if _, ok := tok.(json.Delim); ok {
			return ErrReceiverCall
		}
		return nil
	}
}
