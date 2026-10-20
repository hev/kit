package nativegate

import a "github.com/hev/factory/pkg/nativeadmission"

func dbClaim() a.DBClaim {
	return a.DBClaim{Lease: "db-scope", Holder: "db-peer", LeaseFence: 42, ActionID: "db-action", Claimant: "db-peer", Attempt: 2, ClaimFence: 42}
}
