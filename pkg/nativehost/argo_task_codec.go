package nativehost

import "encoding/json"

// ArgoTaskRPCVersion is separate from the original receiver start/inspect/ACK wire.
// The codec provides no execution endpoint or admission authority.
const ArgoTaskRPCVersion = "native-argo-task-rpc/v1"
const ArgoTaskReferenceLimit = 16384

// ArgoTaskEnvelope routes an opaque reference to a locally pinned profile. The
// optional Factory adapter must parse the task and verify canonical mapping,
// Workflow/node observations and native/DB authority before any effect.
type ArgoTaskEnvelope struct {
	Version       string          `json:"version"`
	ProfileID     string          `json:"profile_id"`
	ConfigDigest  string          `json:"config_digest"`
	TaskReference json.RawMessage `json:"task_reference"`
}

func (e ArgoTaskEnvelope) validate() error {
	if e.Version != ArgoTaskRPCVersion || e.ProfileID == "" || len(e.ProfileID) > 256 || !digest(e.ConfigDigest) || len(e.TaskReference) == 0 || len(e.TaskReference) > ArgoTaskReferenceLimit {
		return ErrHeld
	}
	// Strict framing only; Factory remains the single owner of reference schema.
	var object map[string]json.RawMessage
	if strict(e.TaskReference, &object) != nil || object == nil {
		return ErrHeld
	}
	return nil
}

func DecodeArgoTaskEnvelope(raw []byte) (ArgoTaskEnvelope, error) {
	var e ArgoTaskEnvelope
	if strict(raw, &e) != nil || e.validate() != nil {
		return ArgoTaskEnvelope{}, ErrHeld
	}
	return e, nil
}

func EncodeArgoTaskEnvelope(profileID, configDigest string, task []byte) ([]byte, error) {
	e := ArgoTaskEnvelope{ArgoTaskRPCVersion, profileID, configDigest, append(json.RawMessage(nil), task...)}
	if e.validate() != nil {
		return nil, ErrHeld
	}
	raw, err := json.Marshal(e)
	if err != nil || len(raw) > maxFrame {
		return nil, ErrHeld
	}
	return raw, nil
}
