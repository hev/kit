package filearchive

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/platformops"
)

// The lexical index is an inverted index over chunk text, kept as immutable
// segment files plus a manifest:
//
//	index/manifest.json   segments, and which segment holds each session's
//	                      current postings
//	index/pending         session keys whose chunks changed since the last
//	                      refresh, one per line
//	index/seg-NNNNNN.lx   one segment (format below)
//
// A write only appends the session key to pending. A refresh (run by the
// daemon after each scan and by every search) re-reads those sessions'
// chunks into a new segment and points the manifest at it, so postings for
// the older copy in an earlier segment stop counting. Segments are merged,
// dropping dead postings, when there are too many. Everything happens under
// the archive lock, segment files are never modified after they are written,
// and the manifest is replaced atomically, so a crash leaves either the old
// index or the new one.
//
// Segment format (little-endian):
//
//	"HEVLX01\n"
//	postings  per term: (uvarint doc delta, uvarint tf)...
//	dict      per term, sorted: uvarint len, term, uvarint post offset,
//	          uvarint post bytes, uvarint df
//	sparse    every 32nd dict entry: uvarint len, term, uvarint dict offset
//	docs      per doc, 20 bytes: u32 key index, u32 length, i64 ts (unix ms),
//	          u32 id offset
//	keys      per key: uvarint len, key
//	ids       per doc: uvarint len, chunk id
//	footer    u64 x 9: post, dict, sparse, docs, keys, ids offsets, docs,
//	          keys, total length; then "HEVLXEND"

const (
	segMagic     = "HEVLX01\n"
	segEndMagic  = "HEVLXEND"
	footerSize   = 9*8 + 8
	sparseEvery  = 32
	maxSegments  = 8
	buildDocs    = 100_000
	maxTermBytes = 64
)

var stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields("a an and are as at be but by for from has have how i in is it its of on or that the this to was we were what when where which who why will with you") {
		stopwords[w] = true
	}
}

// terms tokenizes the way the board's filesystem search does: lowercase,
// split on anything that is not a letter or digit, drop stopwords, fold
// simple plurals so "ports" finds "port".
func terms(text string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if stopwords[w] || len(w) > maxTermBytes {
			continue
		}
		out = append(out, fold(w))
	}
	return out
}

func fold(w string) string {
	switch {
	case len(w) > 4 && strings.HasSuffix(w, "ies"):
		return w[:len(w)-3] + "y"
	case len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && !strings.HasSuffix(w, "us"):
		return w[:len(w)-1]
	}
	return w
}

type segInfo struct {
	ID       int   `json:"id"`
	Docs     int   `json:"docs"`
	Live     int   `json:"live"`
	TotalLen int64 `json:"total_len"`
}

type owner struct {
	Seg  int `json:"s"`
	Docs int `json:"d"`
}

type manifest struct {
	Version  int              `json:"version"`
	Next     int              `json:"next"`
	Segments []segInfo        `json:"segments"`
	Owner    map[string]owner `json:"owner"`
}

func (s *Store) indexPath(name string) string { return filepath.Join(s.dir, "index", name) }
func (s *Store) segPath(id int) string        { return s.indexPath(fmt.Sprintf("seg-%06d.lx", id)) }

func (s *Store) loadManifest() (*manifest, error) {
	m := &manifest{Version: 1, Next: 1, Owner: map[string]owner{}}
	b, err := os.ReadFile(s.indexPath("manifest.json"))
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, m); err != nil {
		return nil, fmt.Errorf("archive index manifest: %w", err)
	}
	if m.Owner == nil {
		m.Owner = map[string]owner{}
	}
	return m, nil
}

func (s *Store) saveManifest(m *manifest) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return platformops.WriteFileAtomic(s.indexPath("manifest.json"), b, 0o600)
}

// markDirty records sessions whose chunks changed. The caller holds the lock.
func (s *Store) markDirty(keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	f, err := platformops.OpenFile(s.indexPath("pending"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	for _, k := range keys {
		buf.WriteString(k)
		buf.WriteByte('\n')
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Refresh brings the lexical index up to date with every chunk written so
// far. Search calls it; the daemon calls it after a scan so a search right
// after a big capture does not pay for it.
func (s *Store) Refresh() error {
	return s.locked(s.refresh)
}

// refresh runs under the lock.
func (s *Store) refresh() error {
	raw, err := os.ReadFile(s.indexPath("pending"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	seen := map[string]bool{}
	var pending []string
	for _, line := range strings.Split(string(raw), "\n") {
		if k := strings.TrimSpace(line); k != "" && !seen[k] {
			seen[k] = true
			pending = append(pending, k)
		}
	}
	m, err := s.loadManifest()
	if err != nil {
		return err
	}
	if len(pending) == 0 && len(m.Segments) <= maxSegments {
		return nil
	}
	sort.Strings(pending)
	segs := map[int]*segInfo{}
	for i := range m.Segments {
		segs[m.Segments[i].ID] = &m.Segments[i]
	}
	b := newBuilder()
	flush := func() error {
		if b.empty() {
			return nil
		}
		id := m.Next
		m.Next++
		info, err := b.write(s.segPath(id))
		if err != nil {
			return err
		}
		info.ID = id
		m.Segments = append(m.Segments, info)
		segs = map[int]*segInfo{}
		for i := range m.Segments {
			segs[m.Segments[i].ID] = &m.Segments[i]
		}
		for k, n := range b.keyDocs {
			m.Owner[k] = owner{Seg: id, Docs: n}
		}
		b = newBuilder()
		return nil
	}
	for _, k := range pending {
		if old, ok := m.Owner[k]; ok {
			if seg := segs[old.Seg]; seg != nil {
				seg.Live -= old.Docs
			}
			delete(m.Owner, k)
		}
		rows, err := s.chunkRows(k)
		if err != nil {
			return err
		}
		b.add(k, rows)
		if b.docs() >= buildDocs {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := flush(); err != nil {
		return err
	}
	if err := s.compact(m); err != nil {
		return err
	}
	if err := s.saveManifest(m); err != nil {
		return err
	}
	if err := remove(s.indexPath("pending")); err != nil {
		return err
	}
	return s.sweepSegments(m)
}

// compact drops empty segments and merges when there are too many, or when
// a segment is mostly dead postings.
func (s *Store) compact(m *manifest) error {
	kept := m.Segments[:0]
	for _, seg := range m.Segments {
		if seg.Live > 0 {
			kept = append(kept, seg)
		}
	}
	m.Segments = kept
	var pick []segInfo
	if len(m.Segments) > maxSegments {
		bySize := append([]segInfo(nil), m.Segments...)
		sort.Slice(bySize, func(i, j int) bool { return bySize[i].Live < bySize[j].Live })
		pick = bySize[:len(bySize)-maxSegments/2]
	}
	for _, seg := range m.Segments {
		if seg.Docs >= 1000 && seg.Live*2 < seg.Docs {
			dup := false
			for _, p := range pick {
				dup = dup || p.ID == seg.ID
			}
			if !dup {
				pick = append(pick, seg)
			}
		}
	}
	if len(pick) == 0 || (len(pick) == 1 && pick[0].Live == pick[0].Docs) {
		return nil
	}
	sort.Slice(pick, func(i, j int) bool { return pick[i].ID < pick[j].ID })
	id := m.Next
	m.Next++
	info, err := s.merge(m, pick, s.segPath(id))
	if err != nil {
		return err
	}
	info.ID = id
	picked := map[int]bool{}
	for _, p := range pick {
		picked[p.ID] = true
	}
	kept = m.Segments[:0]
	for _, seg := range m.Segments {
		if !picked[seg.ID] {
			kept = append(kept, seg)
		}
	}
	m.Segments = kept
	if info.Docs > 0 {
		m.Segments = append(m.Segments, info)
	}
	for k, o := range m.Owner {
		if picked[o.Seg] {
			m.Owner[k] = owner{Seg: id, Docs: o.Docs}
		}
	}
	return nil
}

// sweepSegments removes segment files the manifest no longer names.
func (s *Store) sweepSegments(m *manifest) error {
	live := map[string]bool{}
	for _, seg := range m.Segments {
		live[filepath.Base(s.segPath(seg.ID))] = true
	}
	entries, err := os.ReadDir(filepath.Join(s.dir, "index"))
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "seg-") && strings.HasSuffix(name, ".lx") && !live[name] {
			if err := remove(filepath.Join(s.dir, "index", name)); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---- building ----

type posting struct {
	doc uint32
	tf  uint32
}

type docRec struct {
	key uint32
	len uint32
	ts  int64
	id  string
}

type builder struct {
	keys    []string
	keyIdx  map[string]uint32
	keyDocs map[string]int
	docList []docRec
	post    map[string][]posting
	total   int64
}

func newBuilder() *builder {
	return &builder{keyIdx: map[string]uint32{}, keyDocs: map[string]int{}, post: map[string][]posting{}}
}

func (b *builder) empty() bool { return len(b.docList) == 0 }
func (b *builder) docs() int   { return len(b.docList) }

func tsMillis(ts string) int64 {
	if ts == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}

// indexText is what a chunk is searched by: its text plus the tool name, so
// a search for "Bash" finds the tool calls.
func indexText(r layer.Row) string {
	if r.ToolName != "" {
		return r.ToolName + " " + r.Text
	}
	return r.Text
}

func (b *builder) add(key string, rows []layer.Row) {
	if len(rows) == 0 {
		return
	}
	ki, ok := b.keyIdx[key]
	if !ok {
		ki = uint32(len(b.keys))
		b.keyIdx[key] = ki
		b.keys = append(b.keys, key)
	}
	for _, r := range rows {
		doc := uint32(len(b.docList))
		tf := map[string]uint32{}
		n := uint32(0)
		for _, t := range terms(indexText(r)) {
			tf[t]++
			n++
		}
		b.docList = append(b.docList, docRec{key: ki, len: n, ts: tsMillis(r.TS), id: r.ID})
		b.total += int64(n)
		for t, c := range tf {
			b.post[t] = append(b.post[t], posting{doc: doc, tf: c})
		}
		b.keyDocs[key]++
	}
}

func (b *builder) write(path string) (segInfo, error) {
	terms := make([]string, 0, len(b.post))
	for t := range b.post {
		terms = append(terms, t)
	}
	sort.Strings(terms)
	w, err := newSegWriter(path)
	if err != nil {
		return segInfo{}, err
	}
	for _, t := range terms {
		if err := w.term(t, b.post[t]); err != nil {
			w.abort()
			return segInfo{}, err
		}
	}
	if err := w.finish(b.docList, b.keys); err != nil {
		return segInfo{}, err
	}
	return segInfo{Docs: len(b.docList), Live: len(b.docList), TotalLen: b.total}, nil
}

type segWriter struct {
	path    string
	f       *os.File
	w       *bufio.Writer
	off     uint64 // bytes written to f so far
	dict    bytes.Buffer
	sparse  bytes.Buffer
	nTerms  int
	scratch [binary.MaxVarintLen64]byte
}

func newSegWriter(path string) (*segWriter, error) {
	f, err := platformops.CreateTemp(filepath.Dir(path), ".seg-*")
	if err != nil {
		return nil, err
	}
	w := &segWriter{path: path, f: f, w: bufio.NewWriterSize(f, 1<<20)}
	w.w.WriteString(segMagic)
	w.off = uint64(len(segMagic))
	return w, nil
}

func (w *segWriter) abort() {
	w.f.Close()
	os.Remove(w.f.Name())
}

func putUvarint(buf *bytes.Buffer, v uint64) {
	var b [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(b[:], v)
	buf.Write(b[:n])
}

func (w *segWriter) uvarint(v uint64) error {
	n := binary.PutUvarint(w.scratch[:], v)
	_, err := w.w.Write(w.scratch[:n])
	w.off += uint64(n)
	return err
}

func (w *segWriter) term(t string, ps []posting) error {
	if len(ps) == 0 {
		return nil
	}
	start := w.off - uint64(len(segMagic))
	prev := uint32(0)
	for i, p := range ps {
		delta := p.doc - prev
		if i == 0 {
			delta = p.doc
		}
		prev = p.doc
		if err := w.uvarint(uint64(delta)); err != nil {
			return err
		}
		if err := w.uvarint(uint64(p.tf)); err != nil {
			return err
		}
	}
	if w.nTerms%sparseEvery == 0 {
		putUvarint(&w.sparse, uint64(len(t)))
		w.sparse.WriteString(t)
		putUvarint(&w.sparse, uint64(w.dict.Len()))
	}
	putUvarint(&w.dict, uint64(len(t)))
	w.dict.WriteString(t)
	putUvarint(&w.dict, start)
	putUvarint(&w.dict, w.off-uint64(len(segMagic))-start)
	putUvarint(&w.dict, uint64(len(ps)))
	w.nTerms++
	return nil
}

func (w *segWriter) finish(docs []docRec, keys []string) error {
	fail := func(err error) error { w.abort(); return err }
	postOff := uint64(len(segMagic))
	dictOff := w.off
	if _, err := w.w.Write(w.dict.Bytes()); err != nil {
		return fail(err)
	}
	w.off += uint64(w.dict.Len())
	sparseOff := w.off
	if _, err := w.w.Write(w.sparse.Bytes()); err != nil {
		return fail(err)
	}
	w.off += uint64(w.sparse.Len())
	var ids bytes.Buffer
	docsOff := w.off
	rec := make([]byte, 20)
	for _, d := range docs {
		binary.LittleEndian.PutUint32(rec[0:], d.key)
		binary.LittleEndian.PutUint32(rec[4:], d.len)
		binary.LittleEndian.PutUint64(rec[8:], uint64(d.ts))
		binary.LittleEndian.PutUint32(rec[16:], uint32(ids.Len()))
		putUvarint(&ids, uint64(len(d.id)))
		ids.WriteString(d.id)
		if _, err := w.w.Write(rec); err != nil {
			return fail(err)
		}
		w.off += 20
	}
	keysOff := w.off
	var kb bytes.Buffer
	for _, k := range keys {
		putUvarint(&kb, uint64(len(k)))
		kb.WriteString(k)
	}
	if _, err := w.w.Write(kb.Bytes()); err != nil {
		return fail(err)
	}
	w.off += uint64(kb.Len())
	idsOff := w.off
	if _, err := w.w.Write(ids.Bytes()); err != nil {
		return fail(err)
	}
	w.off += uint64(ids.Len())
	footer := make([]byte, footerSize)
	for i, v := range []uint64{postOff, dictOff, sparseOff, docsOff, keysOff, idsOff, uint64(len(docs)), uint64(len(keys)), w.off + footerSize} {
		binary.LittleEndian.PutUint64(footer[i*8:], v)
	}
	copy(footer[72:], segEndMagic)
	if _, err := w.w.Write(footer); err != nil {
		return fail(err)
	}
	if err := w.w.Flush(); err != nil {
		return fail(err)
	}
	if err := w.f.Sync(); err != nil {
		return fail(err)
	}
	if err := w.f.Close(); err != nil {
		os.Remove(w.f.Name())
		return err
	}
	if err := platformops.Rename(w.f.Name(), w.path); err != nil {
		os.Remove(w.f.Name())
		return err
	}
	return nil
}

// ---- reading ----

type sparseEntry struct {
	term string
	off  uint64
}

type segment struct {
	id     int
	f      *os.File
	post   uint64
	dict   uint64
	sparse []sparseEntry
	dictTo uint64
	docs   []byte // 20 bytes per doc
	keys   []string
	ids    uint64
	nDocs  int
}

func openSegment(path string, id int) (*segment, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	seg, err := readSegment(f, id)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("archive index segment %d: %w", id, err)
	}
	return seg, nil
}

func readSegment(f *os.File, id int) (*segment, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := st.Size()
	if size < int64(len(segMagic)+footerSize) {
		return nil, fmt.Errorf("truncated")
	}
	footer := make([]byte, footerSize)
	if _, err := f.ReadAt(footer, size-footerSize); err != nil {
		return nil, err
	}
	if string(footer[72:]) != segEndMagic {
		return nil, fmt.Errorf("bad footer")
	}
	v := make([]uint64, 9)
	for i := range v {
		v[i] = binary.LittleEndian.Uint64(footer[i*8:])
	}
	if v[8] != uint64(size) {
		return nil, fmt.Errorf("length mismatch")
	}
	seg := &segment{id: id, f: f, post: v[0], dict: v[1], dictTo: v[2], ids: v[5], nDocs: int(v[6])}
	read := func(from, to uint64) ([]byte, error) {
		if to < from || to > uint64(size) {
			return nil, fmt.Errorf("bad section bounds")
		}
		b := make([]byte, to-from)
		_, err := f.ReadAt(b, int64(from))
		return b, err
	}
	sparse, err := read(v[2], v[3])
	if err != nil {
		return nil, err
	}
	for r := bytes.NewReader(sparse); r.Len() > 0; {
		t, err := readString(r)
		if err != nil {
			return nil, err
		}
		off, err := binary.ReadUvarint(r)
		if err != nil {
			return nil, err
		}
		seg.sparse = append(seg.sparse, sparseEntry{t, off})
	}
	if seg.docs, err = read(v[3], v[4]); err != nil {
		return nil, err
	}
	if len(seg.docs) != 20*seg.nDocs {
		return nil, fmt.Errorf("doc table size")
	}
	keys, err := read(v[4], v[5])
	if err != nil {
		return nil, err
	}
	for r := bytes.NewReader(keys); r.Len() > 0; {
		k, err := readString(r)
		if err != nil {
			return nil, err
		}
		seg.keys = append(seg.keys, k)
	}
	if len(seg.keys) != int(v[7]) {
		return nil, fmt.Errorf("key table size")
	}
	return seg, nil
}

func readString(r *bytes.Reader) (string, error) {
	n, err := binary.ReadUvarint(r)
	if err != nil {
		return "", err
	}
	if n > uint64(r.Len()) {
		return "", io.ErrUnexpectedEOF
	}
	b := make([]byte, n)
	_, err = io.ReadFull(r, b)
	return string(b), err
}

func (g *segment) close() { g.f.Close() }

func (g *segment) doc(i uint32) docRec {
	b := g.docs[int(i)*20:]
	return docRec{
		key: binary.LittleEndian.Uint32(b[0:]),
		len: binary.LittleEndian.Uint32(b[4:]),
		ts:  int64(binary.LittleEndian.Uint64(b[8:])),
	}
}

func (g *segment) chunkID(i uint32) (string, error) {
	off := uint64(binary.LittleEndian.Uint32(g.docs[int(i)*20+16:]))
	head := make([]byte, binary.MaxVarintLen64)
	n, err := g.f.ReadAt(head, int64(g.ids+off))
	if err != nil && n == 0 {
		return "", err
	}
	l, k := binary.Uvarint(head[:n])
	if k <= 0 {
		return "", fmt.Errorf("bad chunk id")
	}
	b := make([]byte, l)
	if _, err := g.f.ReadAt(b, int64(g.ids+off+uint64(k))); err != nil {
		return "", err
	}
	return string(b), nil
}

type dictEntry struct {
	term          string
	off, size, df uint64
}

// lookup finds a term's dictionary entry.
func (g *segment) lookup(term string) (dictEntry, bool, error) {
	if len(g.sparse) == 0 {
		return dictEntry{}, false, nil
	}
	i := sort.Search(len(g.sparse), func(i int) bool { return g.sparse[i].term > term }) - 1
	if i < 0 {
		return dictEntry{}, false, nil
	}
	from := g.dict + g.sparse[i].off
	to := g.dictTo
	if i+1 < len(g.sparse) {
		to = g.dict + g.sparse[i+1].off
	}
	block := make([]byte, to-from)
	if _, err := g.f.ReadAt(block, int64(from)); err != nil {
		return dictEntry{}, false, err
	}
	r := bytes.NewReader(block)
	for r.Len() > 0 {
		e, err := readDictEntry(r)
		if err != nil {
			return dictEntry{}, false, err
		}
		if e.term == term {
			return e, true, nil
		}
		if e.term > term {
			break
		}
	}
	return dictEntry{}, false, nil
}

func readDictEntry(r *bytes.Reader) (dictEntry, error) {
	var e dictEntry
	var err error
	if e.term, err = readString(r); err != nil {
		return e, err
	}
	if e.off, err = binary.ReadUvarint(r); err != nil {
		return e, err
	}
	if e.size, err = binary.ReadUvarint(r); err != nil {
		return e, err
	}
	e.df, err = binary.ReadUvarint(r)
	return e, err
}

func (g *segment) postings(e dictEntry) ([]posting, error) {
	b := make([]byte, e.size)
	if _, err := g.f.ReadAt(b, int64(g.post+e.off)); err != nil {
		return nil, err
	}
	out := make([]posting, 0, e.df)
	doc := uint64(0)
	for i := 0; len(b) > 0; i++ {
		d, n := binary.Uvarint(b)
		if n <= 0 {
			return nil, fmt.Errorf("bad postings")
		}
		b = b[n:]
		tf, n := binary.Uvarint(b)
		if n <= 0 {
			return nil, fmt.Errorf("bad postings")
		}
		b = b[n:]
		if i == 0 {
			doc = d
		} else {
			doc += d
		}
		out = append(out, posting{doc: uint32(doc), tf: uint32(tf)})
	}
	return out, nil
}

// ---- merging ----

// merge writes the live documents of segs into one new segment.
func (s *Store) merge(m *manifest, segs []segInfo, path string) (segInfo, error) {
	type source struct {
		seg   *segment
		remap []int64 // old doc -> new doc, or -1
		dict  *bytes.Reader
		cur   dictEntry
		done  bool
	}
	var sources []*source
	defer func() {
		for _, src := range sources {
			src.seg.close()
		}
	}()
	var docs []docRec
	var keys []string
	keyIdx := map[string]uint32{}
	var total int64
	for _, info := range segs {
		seg, err := openSegment(s.segPath(info.ID), info.ID)
		if err != nil {
			return segInfo{}, err
		}
		src := &source{seg: seg, remap: make([]int64, seg.nDocs)}
		sources = append(sources, src)
		for i := 0; i < seg.nDocs; i++ {
			d := seg.doc(uint32(i))
			key := seg.keys[d.key]
			if o, ok := m.Owner[key]; !ok || o.Seg != info.ID {
				src.remap[i] = -1
				continue
			}
			id, err := seg.chunkID(uint32(i))
			if err != nil {
				return segInfo{}, err
			}
			ki, ok := keyIdx[key]
			if !ok {
				ki = uint32(len(keys))
				keyIdx[key] = ki
				keys = append(keys, key)
			}
			src.remap[i] = int64(len(docs))
			docs = append(docs, docRec{key: ki, len: d.len, ts: d.ts, id: id})
			total += int64(d.len)
		}
		dict := make([]byte, seg.dictTo-seg.dict)
		if _, err := seg.f.ReadAt(dict, int64(seg.dict)); err != nil {
			return segInfo{}, err
		}
		src.dict = bytes.NewReader(dict)
	}
	if len(docs) == 0 {
		return segInfo{}, nil
	}
	advance := func(src *source) error {
		if src.dict.Len() == 0 {
			src.done = true
			return nil
		}
		e, err := readDictEntry(src.dict)
		src.cur = e
		return err
	}
	for _, src := range sources {
		if err := advance(src); err != nil {
			return segInfo{}, err
		}
	}
	w, err := newSegWriter(path)
	if err != nil {
		return segInfo{}, err
	}
	for {
		term := ""
		found := false
		for _, src := range sources {
			if !src.done && (!found || src.cur.term < term) {
				term, found = src.cur.term, true
			}
		}
		if !found {
			break
		}
		var merged []posting
		for _, src := range sources {
			if src.done || src.cur.term != term {
				continue
			}
			ps, err := src.seg.postings(src.cur)
			if err != nil {
				w.abort()
				return segInfo{}, err
			}
			for _, p := range ps {
				if nd := src.remap[p.doc]; nd >= 0 {
					merged = append(merged, posting{doc: uint32(nd), tf: p.tf})
				}
			}
			if err := advance(src); err != nil {
				w.abort()
				return segInfo{}, err
			}
		}
		if err := w.term(term, merged); err != nil {
			w.abort()
			return segInfo{}, err
		}
	}
	if err := w.finish(docs, keys); err != nil {
		return segInfo{}, err
	}
	return segInfo{Docs: len(docs), Live: len(docs), TotalLen: total}, nil
}

// IndexStats reports the index's size, for status lines and measurements.
type IndexStats struct {
	Segments int
	Docs     int
	Live     int
	Sessions int
	Bytes    int64
}

func (s *Store) IndexStats() (IndexStats, error) {
	var st IndexStats
	err := s.locked(func() error {
		m, err := s.loadManifest()
		if err != nil {
			return err
		}
		st.Segments, st.Sessions = len(m.Segments), len(m.Owner)
		for _, seg := range m.Segments {
			st.Docs += seg.Docs
			st.Live += seg.Live
			if fi, err := os.Stat(s.segPath(seg.ID)); err == nil {
				st.Bytes += fi.Size()
			}
		}
		return nil
	})
	return st, err
}
