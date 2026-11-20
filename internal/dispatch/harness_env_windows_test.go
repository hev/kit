package dispatch

import "testing"

// The Windows CI selector runs Windows-prefixed tests only, so the per-
// harness env, provider and output redaction tests run natively from here.
func TestWindowsHarnessEnv(t *testing.T) {
	for name, test := range map[string]func(*testing.T){
		"launch paths":         TestAgyLaunchPathsInjectSecretEnv,
		"worker fallback":      TestWorkerFallbackAppliesHarnessEnvPerAttempt,
		"round fallback":       TestOrchestratorFallbackAppliesHarnessEnvPerAttempt,
		"plugin-only names":    TestHarnessEnvRefsRejectPluginOnlyNames,
		"secrets unavailable":  TestAgyRunFailsWhenSecretsUnavailable,
		"env name matching":    TestEnvValueFollowsPlatformNameMatching,
		"redaction spans":      TestRedactingWriterSpansWrites,
		"redaction overlap":    TestRedactingWriterOverlappingValues,
		"redaction cap":        TestRedactingWriterCapsHeldOutput,
		"provider args":        TestAgyModelProviderArgs,
		"launch resolves refs": TestHarnessLaunchResolvesEnvRefs,
	} {
		t.Run(name, test)
	}
}
