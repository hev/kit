package dispatch

import "os"

// LocalCapacity reads the receiver's live-process/reservation and FIFO sources.
// It does not list runs, follow forwarding records, or contact another host.
func LocalCapacity() (cap, running, free, queue int, err error) {
	policy, err := loadConfigQuiet("", "", true, true)
	if os.IsNotExist(err) && os.Getenv("HEV_DISPATCH_CONFIG") == "" {
		err = nil
	}
	if err != nil {
		return 0, 0, 0, 0, err
	}
	live, err := liveNames()
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if err = addReservations(live); err != nil {
		return 0, 0, 0, 0, err
	}
	entries, err := queueEntries()
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return policy.Cap, len(live), max(0, policy.Cap-len(live)), len(entries), nil
}
