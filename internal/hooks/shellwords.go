package hooks

import "strings"

// shellLine is a POSIX-shell command line split the way a shell would see
// it, close enough to classify it: simple commands of unquoted words, output
// redirection targets, and the bodies of command substitutions (which run
// too, so the caller classifies them as well).
type shellLine struct {
	commands  [][]string
	redirects []string
	subs      []string
}

// parseShell tokenizes command. It handles single and double quotes,
// backslash escapes, control operators, redirections, comments, here-docs
// and $(...) / `...` substitutions, quoted or not. It never fails: anything
// it cannot make sense of becomes ordinary words.
func parseShell(s string) shellLine {
	var (
		out      shellLine
		words    []string
		cur      strings.Builder
		inWord   bool
		toTarget bool     // the next word is an output redirection target
		toDrop   bool     // the next word is an input file or fd
		heredocs []string // delimiters whose bodies start at the next newline
		heredoc  bool     // the next word is a here-doc delimiter
	)
	flush := func() {
		if !inWord {
			return
		}
		w := cur.String()
		cur.Reset()
		inWord = false
		switch {
		case heredoc:
			heredocs = append(heredocs, w)
			heredoc = false
		case toTarget:
			out.redirects = append(out.redirects, w)
			toTarget = false
		case toDrop:
			toDrop = false
		default:
			words = append(words, w)
		}
	}
	endCommand := func() {
		flush()
		if len(words) > 0 {
			out.commands = append(out.commands, words)
		}
		words = nil
	}
	n := len(s)
	for i := 0; i < n; i++ {
		c := s[i]
		switch c {
		case ' ', '\t', '\r':
			flush()
		case '\n':
			endCommand()
			// Skip here-doc bodies: they are data, not commands.
			for _, d := range heredocs {
				strip := strings.HasPrefix(d, "-")
				d = strings.TrimPrefix(d, "-")
				for i+1 < n {
					end := strings.IndexByte(s[i+1:], '\n')
					line := s[i+1:]
					if end >= 0 {
						line = s[i+1 : i+1+end]
					}
					if end < 0 {
						i = n
					} else {
						i += end + 1
					}
					if strip {
						line = strings.TrimLeft(line, "\t")
					}
					if line == d {
						break
					}
				}
			}
			heredocs = nil
		case ';', '|', '(', ')':
			endCommand()
		case '&':
			if i+1 < n && s[i+1] == '>' {
				// &> and &>> send both streams to a file.
				flush()
				i++
				if i+1 < n && s[i+1] == '>' {
					i++
				}
				toTarget = true
				continue
			}
			endCommand()
		case '#':
			if inWord {
				cur.WriteByte(c)
				continue
			}
			endCommand()
			for i+1 < n && s[i+1] != '\n' {
				i++
			}
		case '\\':
			inWord = true
			if i+1 < n {
				i++
				if s[i] != '\n' {
					cur.WriteByte(s[i])
				}
			}
		case '\'':
			inWord = true
			end := strings.IndexByte(s[i+1:], '\'')
			if end < 0 {
				cur.WriteString(s[i+1:])
				i = n
				continue
			}
			cur.WriteString(s[i+1 : i+1+end])
			i += end + 1
		case '"':
			inWord = true
			for i++; i < n && s[i] != '"'; i++ {
				switch {
				case s[i] == '\\' && i+1 < n && strings.IndexByte("$`\"\\\n", s[i+1]) >= 0:
					i++
					if s[i] != '\n' {
						cur.WriteByte(s[i])
					}
				case s[i] == '$' && i+1 < n && s[i+1] == '(':
					body, j := substitution(s, i+2)
					out.subs = append(out.subs, body)
					cur.WriteString(s[i:j])
					i = j - 1
				case s[i] == '`':
					body, j := backquoted(s, i+1)
					out.subs = append(out.subs, body)
					cur.WriteString(s[i:j])
					i = j - 1
				default:
					cur.WriteByte(s[i])
				}
			}
		case '$':
			inWord = true
			if i+1 < n && s[i+1] == '(' {
				body, j := substitution(s, i+2)
				out.subs = append(out.subs, body)
				cur.WriteString(s[i:j])
				i = j - 1
				continue
			}
			cur.WriteByte(c)
		case '`':
			inWord = true
			body, j := backquoted(s, i+1)
			out.subs = append(out.subs, body)
			cur.WriteString(s[i:j])
			i = j - 1
		case '>', '<':
			// A bare descriptor number before the operator (2>) is not a word.
			if inWord && isDigits(cur.String()) {
				cur.Reset()
				inWord = false
			}
			flush()
			if c == '<' {
				switch {
				case i+1 < n && s[i+1] == '<' && i+2 < n && s[i+2] == '<':
					i += 2 // here-string: the next word is data
					toDrop = true
				case i+1 < n && s[i+1] == '<':
					i++
					if i+1 < n && s[i+1] == '-' {
						i++
						cur.WriteByte('-')
						inWord = true
					}
					heredoc = true
				case i+1 < n && s[i+1] == '(':
					// Process substitution <(...) runs a command.
					body, j := substitution(s, i+2)
					out.subs = append(out.subs, body)
					i = j - 1
				default:
					if i+1 < n && s[i+1] == '&' {
						i++
					}
					toDrop = true
				}
				continue
			}
			if i+1 < n && s[i+1] == '(' {
				body, j := substitution(s, i+2)
				out.subs = append(out.subs, body)
				i = j - 1
				continue
			}
			if i+1 < n && (s[i+1] == '>' || s[i+1] == '|') {
				i++
			}
			if i+1 < n && s[i+1] == '&' {
				// >&N duplicates a descriptor; >&FILE is rare and treated alike.
				i++
				toDrop = true
				continue
			}
			toTarget = true
		default:
			inWord = true
			cur.WriteByte(c)
		}
	}
	endCommand()
	return out
}

// substitution returns the body of a $( ... ) whose body starts at i, and
// the index just past its closing parenthesis.
func substitution(s string, i int) (string, int) {
	depth := 1
	start := i
	for ; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '\'':
			if end := strings.IndexByte(s[i+1:], '\''); end >= 0 {
				i += end + 1
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[start:i], i + 1
			}
		}
	}
	return s[start:], len(s)
}

// backquoted returns the body of a `...` whose body starts at i, and the
// index just past its closing backquote.
func backquoted(s string, i int) (string, int) {
	start := i
	for ; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '`':
			return s[start:i], i + 1
		}
	}
	return s[start:], len(s)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
