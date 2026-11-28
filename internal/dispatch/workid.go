package dispatch

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/hev/kit/pkg/workidentity"
)

// ValidateWorkID accepts a canonical, nonzero UUID without changing its spelling.
func ValidateWorkID(id string) error {
	if _, err := workidentity.ParseUUID(id); err != nil || id == uuid.Nil.String() {
		return fmt.Errorf("invalid work_id: expected a canonical, nonzero UUID")
	}
	return nil
}

// assignWorkID runs only during admission of a new run, including enqueue.
func assignWorkID(m *Meta, declared string) error {
	if m.WorkID != "" {
		if err := ValidateWorkID(m.WorkID); err != nil {
			return err
		}
		if declared != "" && declared != m.WorkID {
			return fmt.Errorf("request work_id conflicts with order work_id")
		}
		return nil
	}
	if declared != "" {
		m.WorkID = declared
		return nil
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return fmt.Errorf("generate work_id: %w", err)
	}
	m.WorkID = id.String()
	return nil
}
