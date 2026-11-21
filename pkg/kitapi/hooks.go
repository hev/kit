package kitapi

const (
	// HookTextRunes bounds each optional field in a merged hook answer.
	HookTextRunes = 10000
	// HookTextWireBytes allows two JSON Unicode escapes per supplementary rune.
	HookTextWireBytes = 12 * HookTextRunes
	HookResultBytes   = 3*HookTextWireBytes + 4096
)
