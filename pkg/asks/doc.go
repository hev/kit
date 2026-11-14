// Package asks renders the read-only asks list and trace waterfall shared by
// local and authenticated servers. Reader adds ListTraceSpanRows to a trace
// archive's session reader because asks are derived from archived dispatch
// facts rather than transcript blocks. Published facts intentionally exclude
// local order bodies and reports.
//
// NewHosted consumes BasePath itself. The application authenticates requests,
// selects a team-scoped Reader and supplies its Principal resolver. Reader
// filters only narrow the team's archive; they must never select a tenant.
// Session may serve the host's existing /api/session/{id} representation for
// inline turns; TracesPath controls links to the host's full session viewer.
// The host must scrub credentials from archived free text before ingestion.
package asks
