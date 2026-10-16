// Package nativejob implements an opt-in, host-local durable process bridge.
// It does not schedule work or activate the trace daemon's execution path.
package nativejob

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

const Version = "native-jobs/v1"

var ErrConflict = errors.New("native job conflict")
var ErrFence = errors.New("expired or stale native job fence")

type Enrollment struct {
	HostID     string `json:"host_id"`
	ExecutorID string `json:"executor_id"`
}
type Identity struct {
	OrderUID          string `json:"order_uid"`
	ScopeDigest       string `json:"scope_digest"`
	PlanDigest        string `json:"plan_digest"`
	AuthorityDigest   string `json:"authority_digest"`
	ExecutionEpoch    string `json:"execution_epoch"`
	WorkflowUID       string `json:"workflow_uid"`
	NodeID            string `json:"node_id"`
	StepID            string `json:"step_id"`
	AttemptID         string `json:"attempt_id"`
	CandidateDigest   string `json:"candidate_digest"`
	InputDigest       string `json:"input_digest"`
	EnvironmentDigest string `json:"environment_digest"`
	Operation         string `json:"operation"`
	HostID            string `json:"host_id"`
	ExecutorID        string `json:"executor_id"`
	ChefID            string `json:"chef_id"`
	Fence             uint64 `json:"fence"`
}
type Fence struct {
	OrderUID string    `json:"order_uid"`
	ChefID   string    `json:"chef_id"`
	Epoch    uint64    `json:"epoch"`
	Expires  time.Time `json:"expires"`
}
type Request struct {
	Version       string   `json:"version"`
	IntentKey     string   `json:"intent_key"`
	PayloadDigest string   `json:"payload_digest"`
	Identity      Identity `json:"identity"`
	Argv          []string `json:"argv"`
	Directory     string   `json:"directory"`
	// The complete child environment, not additions to the daemon environment.
	Environment []string `json:"environment"`
}
type Birth struct {
	BootID string `json:"boot_id"`
	PID    int    `json:"pid"`
	Start  string `json:"start"`
}
type Artifact struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}
type Result struct {
	ExitCode  int        `json:"exit_code"`
	Artifacts []Artifact `json:"artifacts"`
	Evidence  string     `json:"evidence,omitempty"`
}
type Inspection struct {
	ID        string `json:"id"`
	Evidence  string `json:"evidence"`
	Inspector string `json:"inspector"`
	Result    Result `json:"result"`
}
type Job struct {
	ID           string       `json:"id"`
	Request      Request      `json:"request"`
	State        string       `json:"state"` // Pending, Uncertain, Running, Completed
	Birth        *Birth       `json:"birth,omitempty"`
	Owner        *Birth       `json:"owner,omitempty"`
	Result       *Result      `json:"result,omitempty"`
	ResultDigest string       `json:"result_digest,omitempty"`
	EventID      string       `json:"event_id,omitempty"`
	Acknowledged bool         `json:"acknowledged"`
	Inspections  []Inspection `json:"inspections,omitempty"`
}
type Event struct {
	Version      string   `json:"version"`
	EventID      string   `json:"event_id"`
	JobID        string   `json:"job_id"`
	Identity     Identity `json:"identity"`
	Result       Result   `json:"result"`
	ResultDigest string   `json:"result_digest"`
}

func Digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

// RequestDigest binds every execution field, including the complete identity.
func RequestDigest(r Request) string { r.PayloadDigest = ""; return Digest(r) }
func (r Request) validate(e Enrollment) error {
	if r.Version != Version || r.IntentKey == "" || strings.HasPrefix(r.IntentKey, "job-") || r.PayloadDigest != RequestDigest(r) {
		return errors.New("invalid version, intent key or payload digest")
	}
	v := reflect.ValueOf(r.Identity)
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).Kind() == reflect.String && v.Field(i).String() == "" {
			return fmt.Errorf("missing identity %s", v.Type().Field(i).Name)
		}
	}
	if r.Identity.HostID != e.HostID || r.Identity.ExecutorID != e.ExecutorID {
		return errors.New("executor enrollment mismatch")
	}
	if r.Identity.Fence == 0 || r.Identity.Operation != "exec" || len(r.Argv) == 0 || r.Argv[0] == "" || r.Directory == "" {
		return errors.New("invalid exec operation")
	}
	return nil
}
