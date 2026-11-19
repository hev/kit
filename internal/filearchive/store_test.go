package filearchive

import "github.com/hev/kit/internal/archive"

var (
	_ archive.Store         = (*Store)(nil)
	_ archive.RegexSearcher = (*Store)(nil)
)
