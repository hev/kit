package kitapi

const (
	// HookTextRunes bounds each optional field in a merged hook answer.
	HookTextRunes = 10000
	// HookResultBytes allows six JSON wire bytes per rune (escaped control
	// characters) plus the envelope and event identifier.
	HookResultBytes = 3*HookTextRunes*6 + 4096
)
