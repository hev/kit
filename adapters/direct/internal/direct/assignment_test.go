package direct

import "testing"

func TestAssignmentBindsValidationCandidateAndLimits(t *testing.T) {
	a := Row{"role": "implementation", "model": "selected-model", "source_revision": "0123456789012345678901234567890123456789", "directory": "/source", "brief": "bounded; relevant skill content and acceptance included", "max_seconds": float64(30)}
	if err := ValidateBrief(a); err != nil {
		t.Fatal(err)
	}
	a["role"] = "validation"
	if ValidateBrief(a) == nil {
		t.Fatal("validation without implementation identity accepted")
	}
	a["implementation_run"] = "run-a"
	if err := ValidateBrief(a); err != nil {
		t.Fatal(err)
	}
	a["max_seconds"] = float64(0)
	if ValidateBrief(a) == nil {
		t.Fatal("unbounded execution accepted")
	}
}
