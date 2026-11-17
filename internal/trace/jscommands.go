package trace

// javascriptCommandStrings returns the shell scripts code-mode JavaScript
// passes to tools.exec_command, and whether the code calls it at all.
//
// A cmd (or command) value counts when it is a string literal (a template
// literal keeps its fixed words) or when it resolves, through the binding
// of the name the call actually uses, to string literals:
//
//   - const c = "…"                                  then {cmd: c} or {cmd}
//   - for (const c of [list]) / for (const [k, o] of calls)
//   - [list].map(c => …) / names.map(async (c, i) => …) /
//     calls.map(([name, c]) => …), any depth of array destructuring
//     (also forEach and flatMap)
//   - const o = {cmd: "…"}                           then tools.exec_command(o)
//   - {cmd: o.cmd} over any of the above.
//
// Only the nearest binding of that name before the call is followed, and a
// list is a literal array of literals (or of objects, or of tuples when the
// loop destructures). Anything else, such as a computed value, a function
// parameter or a value built with an operator, stays unknown and counts as
// nothing; a later reassignment is not followed, so it can undercount but
// never turns data into a command. Other strings in the script are data,
// whatever they say.
func javascriptCommandStrings(code string) ([]string, bool) {
	r := jsResolver{jsTokens(code, true)}
	var commands []string
	wrapped := false
	for i := 0; i+4 < len(r.t); i++ {
		if !r.punct(i, "tools") || !r.punct(i+1, ".") || !r.punct(i+2, "exec_command") || !r.punct(i+3, "(") {
			continue
		}
		wrapped = true
		a := i + 4
		switch {
		case r.punct(a, "{"):
			commands = append(commands, r.objectCommands(a, i)...)
		case r.ident(a) && r.punct(a+1, ")"):
			commands = append(commands, r.nameObjectCommands(r.t[a].text, i)...)
		}
	}
	return commands, wrapped
}

type jsResolver struct{ t []jsSkillToken }

func (r jsResolver) punct(i int, v string) bool {
	return i >= 0 && i < len(r.t) && !r.t[i].literal && r.t[i].text == v
}

func (r jsResolver) lit(i int) bool { return i >= 0 && i < len(r.t) && r.t[i].literal }

func (r jsResolver) ident(i int) bool {
	if i < 0 || i >= len(r.t) || r.t[i].literal || r.t[i].text == "" {
		return false
	}
	c := r.t[i].text[0]
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func (r jsResolver) declKeyword(i int) bool {
	return r.punct(i, "const") || r.punct(i, "let") || r.punct(i, "var")
}

// elements splits the bracketed literal opening at i ({ or [) into the
// token ranges of its top-level comma-separated entries.
func (r jsResolver) elements(i int) [][2]int {
	var out [][2]int
	depth, start := 0, i+1
	for j := i; j < len(r.t); j++ {
		if r.t[j].literal {
			continue
		}
		switch r.t[j].text {
		case "{", "[", "(":
			depth++
		case "}", "]", ")":
			depth--
			if depth == 0 {
				if j > start {
					out = append(out, [2]int{start, j})
				}
				return out
			}
		case ",":
			if depth == 1 {
				out = append(out, [2]int{start, j})
				start = j + 1
			}
		}
	}
	return nil // unterminated
}

// open returns the index of the bracket that the closing bracket at i closes.
func (r jsResolver) open(i int) int {
	depth := 0
	for j := i; j >= 0; j-- {
		if r.t[j].literal {
			continue
		}
		switch r.t[j].text {
		case "}", "]", ")":
			depth++
		case "{", "[", "(":
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

// objectCommands reads the cmd/command property of the object literal at i;
// at is the call site, for resolving names.
func (r jsResolver) objectCommands(i, at int) []string {
	var out []string
	for _, e := range r.elements(i) {
		s, end := e[0], e[1]
		if !r.punct(s, "cmd") && !r.punct(s, "command") {
			continue
		}
		switch {
		case end-s == 1: // shorthand {cmd}
			out = append(out, r.nameStrings(r.t[s].text, at)...)
		case r.punct(s+1, ":"):
			out = append(out, r.valueStrings(s+2, end, at)...)
		}
	}
	return out
}

// literalObjectCommands reads a literal cmd/command property of the object
// literal at i, without resolving names.
func (r jsResolver) literalObjectCommands(i int) []string {
	var out []string
	for _, e := range r.elements(i) {
		s, end := e[0], e[1]
		if (r.punct(s, "cmd") || r.punct(s, "command")) && r.punct(s+1, ":") && end-s == 3 && r.lit(s+2) {
			out = append(out, r.t[s+2].text)
		}
	}
	return out
}

// valueStrings resolves the expression in [s, end) to string literals.
func (r jsResolver) valueStrings(s, end, at int) []string {
	switch {
	case end-s == 1 && r.lit(s):
		return []string{r.t[s].text}
	case end-s == 1 && r.ident(s):
		return r.nameStrings(r.t[s].text, at)
	case end-s == 3 && r.ident(s) && r.punct(s+1, ".") && (r.punct(s+2, "cmd") || r.punct(s+2, "command")):
		return r.nameObjectCommands(r.t[s].text, at)
	}
	return nil
}

// jsBinding is where a name gets its value: a declaration's initializer, or
// the elements of a literal list it iterates. path is the name's position in
// an array destructuring pattern ([k, [name]] is 1, 0), empty for a plain name.
type jsBinding struct {
	value int // initializer start, or -1
	list  int // index of the iterated list's "[", or -1
	path  []int
}

// binding finds the nearest binding of name before at.
func (r jsResolver) binding(name string, at int) (jsBinding, bool) {
	for k := at - 1; k >= 0; k-- {
		if !r.punct(k, name) {
			continue
		}
		// const name = value
		if r.declKeyword(k-1) && r.punct(k+1, "=") && !r.punct(k+2, "=") && !r.punct(k+2, ">") {
			return jsBinding{value: k + 2, list: -1}, true
		}
		path, root, ok := r.patternPath(k)
		if !ok {
			continue
		}
		end := root
		if len(path) > 0 {
			end = r.closeOf(root)
		}
		// for (const name of list), for (const [a, [name]] of list)
		if r.declKeyword(root-1) && r.punct(end+1, "of") {
			return jsBinding{value: -1, list: r.listAt(end+2, root), path: path}, true
		}
		// list.map(name => …), list.map(async ([k, name], i) => …)
		if b, ok := r.arrowParam(root, end); ok {
			b.path = path
			return b, true
		}
	}
	return jsBinding{}, false
}

// patternPath climbs from the name at k through the array destructuring
// patterns around it, returning its index path and the outermost pattern's
// "[" (k itself when it is not in a pattern).
func (r jsResolver) patternPath(k int) ([]int, int, bool) {
	var path []int
	for {
		e := r.enclosing(k)
		if !r.punct(e, "[") {
			return path, k, true
		}
		idx, depth := 0, 0
		for j := e + 1; j < k; j++ {
			switch {
			case r.punct(j, "[") || r.punct(j, "{"):
				depth++
			case r.punct(j, "]") || r.punct(j, "}"):
				depth--
			case r.punct(j, ","):
				if depth == 0 {
					idx++
				}
			case r.ident(j):
			default:
				return nil, 0, false // not a pattern
			}
		}
		path = append([]int{idx}, path...)
		k = e
	}
}

// arrowParam binds the parameter whose tokens span [start, end] as the
// element parameter of a list callback: list.map(p => …) or
// list.map(async (p, i) => …), for map, forEach and flatMap.
func (r jsResolver) arrowParam(start, end int) (jsBinding, bool) {
	var p int
	if start == end && r.punct(end+1, "=") && r.punct(end+2, ">") {
		p = start - 1 // name => …
	} else {
		e := r.enclosing(start)
		if !r.punct(e, "(") || e+1 != start { // the first parameter only
			return jsBinding{}, false
		}
		c := r.closeOf(e)
		if c < 0 || !r.punct(c+1, "=") || !r.punct(c+2, ">") {
			return jsBinding{}, false
		}
		p = e - 1
	}
	if r.punct(p, "async") {
		p--
	}
	if !r.punct(p, "(") || !r.punct(p-2, ".") || !(r.punct(p-1, "map") || r.punct(p-1, "forEach") || r.punct(p-1, "flatMap")) {
		return jsBinding{}, false
	}
	return jsBinding{value: -1, list: r.listAt(p-3, p-3)}, true
}

// enclosing returns the index of the innermost bracket open at token k.
func (r jsResolver) enclosing(k int) int {
	depth := 0
	for j := k - 1; j >= 0; j-- {
		if r.t[j].literal {
			continue
		}
		switch r.t[j].text {
		case "}", "]", ")":
			depth++
		case "{", "[", "(":
			if depth == 0 {
				return j
			}
			depth--
		}
	}
	return -1
}

// listAt resolves the list expression at i (a literal "[…]" starting or
// ending there, or a name declared as one) to the index of its "[".
func (r jsResolver) listAt(i, at int) int {
	switch {
	case r.punct(i, "["):
		return i
	case r.punct(i, "]"):
		if o := r.open(i); r.punct(o, "[") {
			return o
		}
	case r.ident(i):
		if b, ok := r.binding(r.t[i].text, at); ok && b.value >= 0 && r.punct(b.value, "[") {
			return b.value
		}
	}
	return -1
}

// listEntries returns the ranges a list binding hands its name, one per
// iteration, following its destructuring path.
func (r jsResolver) listEntries(b jsBinding) [][2]int {
	if b.list < 0 {
		return nil
	}
	var out [][2]int
next:
	for _, e := range r.elements(b.list) {
		for _, i := range b.path {
			if !r.punct(e[0], "[") || r.closeOf(e[0]) != e[1]-1 {
				continue next
			}
			inner := r.elements(e[0])
			if i >= len(inner) {
				continue next
			}
			e = inner[i]
		}
		out = append(out, e)
	}
	return out
}

// nameStrings resolves name, used as a cmd value at the call site at, to
// the string literals it holds.
func (r jsResolver) nameStrings(name string, at int) []string {
	b, ok := r.binding(name, at)
	if !ok {
		return nil
	}
	if b.value >= 0 {
		if r.lit(b.value) && r.statementEnds(b.value+1) {
			return []string{r.t[b.value].text}
		}
		return nil
	}
	var out []string
	for _, e := range r.listEntries(b) {
		if e[1]-e[0] == 1 && r.lit(e[0]) {
			out = append(out, r.t[e[0]].text)
		}
	}
	return out
}

// nameObjectCommands resolves name, used as a whole tools.exec_command
// argument (or as name.cmd), to the literal cmd values of the objects it holds.
func (r jsResolver) nameObjectCommands(name string, at int) []string {
	b, ok := r.binding(name, at)
	if !ok {
		return nil
	}
	if b.value >= 0 {
		if r.punct(b.value, "{") {
			if c := r.closeOf(b.value); c >= 0 && r.statementEnds(c+1) {
				return r.literalObjectCommands(b.value)
			}
		}
		return nil
	}
	var out []string
	for _, e := range r.listEntries(b) {
		if r.punct(e[0], "{") && r.closeOf(e[0]) == e[1]-1 {
			out = append(out, r.literalObjectCommands(e[0])...)
		}
	}
	return out
}

// closeOf returns the index of the bracket closing the one at i.
func (r jsResolver) closeOf(i int) int {
	depth := 0
	for j := i; j < len(r.t); j++ {
		if r.t[j].literal {
			continue
		}
		switch r.t[j].text {
		case "{", "[", "(":
			depth++
		case "}", "]", ")":
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

// statementEnds reports that an initializer ends before token i: the
// value is used as written, not combined with an operator or call.
func (r jsResolver) statementEnds(i int) bool {
	return i >= len(r.t) || r.punct(i, ";") || r.punct(i, ",") || r.punct(i, "}") || r.punct(i, ")") || r.ident(i)
}
