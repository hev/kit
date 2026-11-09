package compat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Obj is a JSON object that keeps insertion order, as Python dicts do, so the
// bytes written here match json.dumps in the reference implementation.
type Obj struct {
	keys []string
	vals map[string]any
}

func NewObj(kv ...any) *Obj {
	o := &Obj{vals: map[string]any{}}
	for i := 0; i+1 < len(kv); i += 2 {
		o.Set(kv[i].(string), kv[i+1])
	}
	return o
}

func (o *Obj) Set(k string, v any) *Obj {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
	return o
}

func (o *Obj) Get(k string) any {
	if o == nil {
		return nil
	}
	return o.vals[k]
}

func (o *Obj) Has(k string) bool {
	if o == nil {
		return false
	}
	_, ok := o.vals[k]
	return ok
}

func (o *Obj) Del(k string) {
	if !o.Has(k) {
		return
	}
	delete(o.vals, k)
	for i, key := range o.keys {
		if key == k {
			o.keys = append(o.keys[:i:i], o.keys[i+1:]...)
			break
		}
	}
}

// Pop removes k and returns its value (Python dict.pop(k, None)).
func (o *Obj) Pop(k string) any {
	v := o.Get(k)
	o.Del(k)
	return v
}

func (o *Obj) Keys() []string {
	if o == nil {
		return nil
	}
	return append([]string(nil), o.keys...)
}

func (o *Obj) Len() int {
	if o == nil {
		return 0
	}
	return len(o.keys)
}

// Merge returns {**o, **others...}.
func (o *Obj) Merge(others ...*Obj) *Obj {
	out := NewObj()
	for _, src := range append([]*Obj{o}, others...) {
		for _, k := range src.Keys() {
			out.Set(k, src.vals[k])
		}
	}
	return out
}

func (o *Obj) Copy() *Obj { return o.Merge() }

func (o *Obj) Str(k string) string {
	s, _ := o.Get(k).(string)
	return s
}

func (o *Obj) Obj(k string) *Obj {
	v, _ := o.Get(k).(*Obj)
	return v
}

func (o *Obj) List(k string) []any {
	v, _ := o.Get(k).([]any)
	return v
}

// Num returns a numeric field as float64 (Python int or float).
func (o *Obj) Num(k string) (float64, bool) { return num(o.Get(k)) }

func num(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

// Truthy is Python's bool(v).
func Truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case *Obj:
		return x.Len() > 0
	default:
		f, ok := num(v)
		return !ok || f != 0
	}
}

// Equal is Python ==, including 1 == 1.0.
func Equal(a, b any) bool {
	if fa, ok := num(a); ok {
		if _, isBool := a.(bool); !isBool {
			fb, ok := num(b)
			return ok && fa == fb
		}
	}
	switch x := a.(type) {
	case nil:
		return b == nil
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !Equal(x[i], y[i]) {
				return false
			}
		}
		return true
	case *Obj:
		y, ok := b.(*Obj)
		if !ok || x.Len() != y.Len() {
			return false
		}
		for _, k := range x.keys {
			if !y.Has(k) || !Equal(x.vals[k], y.vals[k]) {
				return false
			}
		}
		return true
	}
	return false
}

// Loads is json.loads: object order kept, ints stay ints, floats are float64,
// and NaN/Infinity accepted. Strings are decoded here rather than by
// encoding/json, which turns a lone surrogate escape such as "\udcff" (a
// Python surrogateescape path byte) into U+FFFD. Lone surrogates are kept as
// their WTF-8 bytes instead, and quote writes them back as the same escape.
func Loads(b []byte) (any, error) {
	d := &decoder{b: b}
	d.space()
	v, err := d.value()
	if err != nil {
		return nil, err
	}
	if d.space(); d.i != len(b) {
		return nil, d.errorf("extra data")
	}
	return v, nil
}

// LoadObj parses a JSON object; anything else is an error.
func LoadObj(b []byte) (*Obj, error) {
	v, err := Loads(b)
	if err != nil {
		return nil, err
	}
	o, ok := v.(*Obj)
	if !ok {
		return nil, fmt.Errorf("not an object")
	}
	return o, nil
}

type decoder struct {
	b []byte
	i int
}

func (d *decoder) errorf(format string, args ...any) error {
	return fmt.Errorf("json: %s at char %d", fmt.Sprintf(format, args...), d.i)
}

func (d *decoder) space() {
	for d.i < len(d.b) && strings.IndexByte(" \t\n\r", d.b[d.i]) >= 0 {
		d.i++
	}
}

func (d *decoder) literal(word string) bool {
	if bytes.HasPrefix(d.b[d.i:], []byte(word)) {
		d.i += len(word)
		return true
	}
	return false
}

func (d *decoder) value() (any, error) {
	if d.i >= len(d.b) {
		return nil, d.errorf("expecting value")
	}
	switch c := d.b[d.i]; {
	case c == '{':
		return d.object()
	case c == '[':
		return d.array()
	case c == '"':
		return d.str()
	case d.literal("null"):
		return nil, nil
	case d.literal("true"):
		return true, nil
	case d.literal("false"):
		return false, nil
	case d.literal("NaN"):
		return math.NaN(), nil
	case d.literal("Infinity"):
		return math.Inf(1), nil
	case d.literal("-Infinity"):
		return math.Inf(-1), nil
	case c == '-' || c >= '0' && c <= '9':
		return d.number()
	}
	return nil, d.errorf("expecting value")
}

func (d *decoder) object() (any, error) {
	o := NewObj()
	d.i++
	d.space()
	if d.i < len(d.b) && d.b[d.i] == '}' {
		d.i++
		return o, nil
	}
	for {
		if d.i >= len(d.b) || d.b[d.i] != '"' {
			return nil, d.errorf("expecting property name enclosed in double quotes")
		}
		k, err := d.str()
		if err != nil {
			return nil, err
		}
		if d.space(); d.i >= len(d.b) || d.b[d.i] != ':' {
			return nil, d.errorf("expecting ':' delimiter")
		}
		d.i++
		d.space()
		v, err := d.value()
		if err != nil {
			return nil, err
		}
		o.Set(k, v)
		d.space()
		if d.i < len(d.b) && d.b[d.i] == '}' {
			d.i++
			return o, nil
		}
		if d.i >= len(d.b) || d.b[d.i] != ',' {
			return nil, d.errorf("expecting ',' delimiter")
		}
		d.i++
		d.space()
	}
}

func (d *decoder) array() (any, error) {
	l := []any{}
	d.i++
	d.space()
	if d.i < len(d.b) && d.b[d.i] == ']' {
		d.i++
		return l, nil
	}
	for {
		v, err := d.value()
		if err != nil {
			return nil, err
		}
		l = append(l, v)
		d.space()
		if d.i < len(d.b) && d.b[d.i] == ']' {
			d.i++
			return l, nil
		}
		if d.i >= len(d.b) || d.b[d.i] != ',' {
			return nil, d.errorf("expecting ',' delimiter")
		}
		d.i++
		d.space()
	}
}

var numberRE = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(\.[0-9]+)?([eE][-+]?[0-9]+)?`)

func (d *decoder) number() (any, error) {
	m := numberRE.FindSubmatchIndex(d.b[d.i:])
	if m == nil {
		return nil, d.errorf("expecting value")
	}
	s := string(d.b[d.i : d.i+m[1]])
	d.i += m[1]
	if m[2] >= 0 || m[4] >= 0 {
		// Python float() overflows to inf rather than failing.
		f, err := strconv.ParseFloat(s, 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			return nil, err
		}
		return f, nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return int(n), nil
	}
	return json.Number(s), nil
}

// str decodes a JSON string as Python's strict scanner does. A surrogate pair
// becomes one code point; a lone surrogate becomes its WTF-8 encoding.
func (d *decoder) str() (string, error) {
	d.i++
	var b []byte
	for {
		if d.i >= len(d.b) {
			return "", d.errorf("unterminated string")
		}
		c := d.b[d.i]
		switch {
		case c == '"':
			d.i++
			return string(b), nil
		case c < 0x20:
			return "", d.errorf("invalid control character")
		case c != '\\':
			b = append(b, c)
			d.i++
			continue
		}
		if d.i+1 >= len(d.b) {
			return "", d.errorf("unterminated string")
		}
		esc := d.b[d.i+1]
		d.i += 2
		if r, ok := map[byte]byte{'"': '"', '\\': '\\', '/': '/', 'b': '\b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t'}[esc]; ok {
			b = append(b, r)
			continue
		}
		if esc != 'u' {
			return "", d.errorf("invalid \\escape")
		}
		r, err := d.hex4()
		if err != nil {
			return "", err
		}
		if utf16.IsSurrogate(r) && r < 0xdc00 && bytes.HasPrefix(d.b[d.i:], []byte(`\u`)) {
			save := d.i
			d.i += 2
			if lo, err := d.hex4(); err == nil && lo >= 0xdc00 && lo <= 0xdfff {
				b = utf8.AppendRune(b, utf16.DecodeRune(r, lo))
				continue
			}
			d.i = save
		}
		b = appendWTF8(b, r)
	}
}

func (d *decoder) hex4() (rune, error) {
	if d.i+4 > len(d.b) {
		return 0, d.errorf("invalid \\uXXXX escape")
	}
	n, err := strconv.ParseUint(string(d.b[d.i:d.i+4]), 16, 32)
	if err != nil {
		return 0, d.errorf("invalid \\uXXXX escape")
	}
	d.i += 4
	return rune(n), nil
}

// appendWTF8 is utf8.AppendRune that also encodes surrogate code points
// (WTF-8), which utf8 replaces with U+FFFD.
func appendWTF8(b []byte, r rune) []byte {
	if !utf16.IsSurrogate(r) {
		return utf8.AppendRune(b, r)
	}
	return append(b, 0xe0|byte(r>>12), 0x80|byte(r>>6)&0x3f, 0x80|byte(r)&0x3f)
}

// decodeWTF8 is utf8.DecodeRuneInString that also reads a WTF-8 surrogate.
// Any other invalid byte b is reported as surrogateescape's U+DC00+b, which
// is how Python sees non-UTF-8 bytes from the OS (paths, argv, environment).
func decodeWTF8(s string) (rune, int) {
	r, n := utf8.DecodeRuneInString(s)
	if r != utf8.RuneError || n != 1 {
		return r, n
	}
	if len(s) >= 3 && s[0] == 0xed && s[1] >= 0xa0 && s[1] <= 0xbf && s[2] >= 0x80 && s[2] <= 0xbf {
		return 0xd000 | rune(s[1]&0x3f)<<6 | rune(s[2]&0x3f), 3
	}
	if s[0] < 0x80 {
		return r, n
	}
	return 0xdc00 + rune(s[0]), 1
}

// Dumps is json.dumps(v) (indent < 0) or json.dumps(v, indent=n), ensure_ascii.
func Dumps(v any, indent ...int) string {
	var b strings.Builder
	n := -1
	if len(indent) > 0 {
		n = indent[0]
	}
	encode(&b, v, n, 0, true)
	return b.String()
}

// DumpsUnicode is json.dumps(v, ensure_ascii=False).
func DumpsUnicode(v any) string {
	var b strings.Builder
	encode(&b, v, -1, 0, false)
	return b.String()
}

func encode(b *strings.Builder, v any, indent, level int, ascii bool) {
	newline := func(l int) {
		if indent >= 0 {
			b.WriteByte('\n')
			b.WriteString(strings.Repeat(" ", indent*l))
		}
	}
	sep := ", "
	if indent >= 0 {
		sep = ","
	}
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case string:
		b.WriteString(quote(x, ascii))
	case int:
		b.WriteString(strconv.Itoa(x))
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case json.Number:
		b.WriteString(string(x))
	case float64:
		b.WriteString(FloatRepr(x))
	case []string:
		l := make([]any, len(x))
		for i := range x {
			l[i] = x[i]
		}
		encode(b, l, indent, level, ascii)
	case [][]string:
		l := make([]any, len(x))
		for i := range x {
			l[i] = x[i]
		}
		encode(b, l, indent, level, ascii)
	case []*Obj:
		l := make([]any, len(x))
		for i := range x {
			l[i] = x[i]
		}
		encode(b, l, indent, level, ascii)
	case []any:
		if len(x) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i, item := range x {
			if i > 0 {
				b.WriteString(sep)
			}
			newline(level + 1)
			encode(b, item, indent, level+1, ascii)
		}
		newline(level)
		b.WriteByte(']')
	case *Obj:
		if x == nil {
			b.WriteString("null")
			return
		}
		if x.Len() == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteByte('{')
		for i, k := range x.keys {
			if i > 0 {
				b.WriteString(sep)
			}
			newline(level + 1)
			b.WriteString(quote(k, ascii))
			b.WriteString(": ")
			encode(b, x.vals[k], indent, level+1, ascii)
		}
		newline(level)
		b.WriteByte('}')
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		o := NewObj()
		for _, k := range keys {
			o.Set(k, x[k])
		}
		encode(b, o, indent, level, ascii)
	default:
		panic(fmt.Sprintf("pyjson: unsupported %T", v))
	}
}

func quote(s string, ascii bool) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); {
		r, n := decodeWTF8(s[i:])
		raw := s[i : i+n]
		i += n
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			switch {
			case r < 0x20:
				fmt.Fprintf(&b, `\u%04x`, r)
			case r < 0x7f || !ascii:
				// Unescaped output keeps surrogates and stray bytes as they
				// were, for a later ensure_ascii pass to escape.
				b.WriteString(raw)
			case r > 0xffff:
				a, c := utf16.EncodeRune(r)
				fmt.Fprintf(&b, `\u%04x\u%04x`, a, c)
			default:
				fmt.Fprintf(&b, `\u%04x`, r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// FloatRepr is Python's repr(float).
func FloatRepr(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	}
	s := strconv.FormatFloat(f, 'e', -1, 64) // d.ddde±XX
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	mant, expText, _ := strings.Cut(s, "e")
	exp, _ := strconv.Atoi(expText)
	digits := strings.Replace(mant, ".", "", 1)
	var out string
	if exp >= -4 && exp < 16 {
		if exp >= 0 {
			if len(digits) <= exp+1 {
				out = digits + strings.Repeat("0", exp+1-len(digits)) + ".0"
			} else {
				out = digits[:exp+1] + "." + digits[exp+1:]
			}
		} else {
			out = "0." + strings.Repeat("0", -exp-1) + digits
		}
	} else {
		out = digits[:1]
		if len(digits) > 1 {
			out += "." + digits[1:]
		}
		sign := "+"
		if exp < 0 {
			sign, exp = "-", -exp
		}
		out += fmt.Sprintf("e%s%02d", sign, exp)
	}
	if neg {
		out = "-" + out
	}
	return out
}
