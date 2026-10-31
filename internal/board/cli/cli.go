package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hev/kit/internal/board/layer"
	"github.com/hev/kit/internal/board/server"
	"github.com/hev/kit/internal/board/settings"
	"github.com/hev/kit/internal/board/store"
	"github.com/hev/kit/internal/board/who"
)

func Run(args []string) error {
	if err := settings.Load(); err != nil {
		return err
	}
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}
	switch cmd {
	case "-h", "--help", "help":
		fmt.Print(usage)
		return nil
	case "serve":
		return runServe(args)
	case "", "ls", "list":
		return runList(args)
	case "boards":
		return runBoards(args)
	case "read", "show":
		return runRead(args)
	case "search", "find":
		return runSearch(args)
	case "post", "new":
		return runPost(args)
	case "reply":
		return runReply(args)
	case "useful":
		return runUseful(args)
	case "pin", "unpin", "lock", "unlock":
		return runFlag(cmd, args)
	case "whoami":
		return runWhoami(args)
	case "canvas":
		return runCanvas(args)
	case "open":
		return runOpen()
	case "url":
		fmt.Println(baseURL())
		return nil
	}
	return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", settings.Current.Addr, "listen address")
	ns := fs.String("namespace", "", "Layer namespace (default $FACTORY_BOARD_NAMESPACE or "+settings.Current.Namespace+")")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ns != "" {
		settings.Current.Namespace = *ns
	}
	if *addr == "" {
		return errors.New("configure board.addr or pass --addr")
	}
	cl, err := layer.Load()
	if err != nil {
		return err
	}
	srv, err := server.New(store.New(cl))
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	hs := &http.Server{Handler: logged(srv.Handler()), ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(shut)
	}()
	fmt.Fprintf(os.Stderr, "hev board: serving http://%s from %s/%s\n", ln.Addr(), cl.Endpoint, cl.Namespace)
	if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func logged(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		h.ServeHTTP(w, r)
		if !strings.HasPrefix(r.URL.Path, "/static/") {
			fmt.Fprintf(os.Stderr, "%s %s %s %s\n", start.UTC().Format(time.RFC3339), r.Method, r.URL.RequestURI(), time.Since(start).Round(time.Millisecond))
		}
	})
}

func baseURL() string { return strings.TrimRight(settings.Current.URL, "/") }

var client = &http.Client{Timeout: 10 * time.Second}

func call(method, path string, in, out any) error {
	if baseURL() == "" {
		return errors.New("configure board.url or FACTORY_BOARD_URL")
	}
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, baseURL()+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("board not reachable at %s (%v) — is it serving? set FACTORY_BOARD_URL if it lives elsewhere", baseURL(), err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error == "" {
			e.Error = strings.TrimSpace(string(raw))
		}
		return errors.New(e.Error)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

type common struct {
	fs   *flag.FlagSet
	as   *string
	json *bool
}

func flags(name string) common {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	return common{
		fs:   fs,
		as:   fs.String("as", "", "post as this handle (default: the tmux session, else $USER)"),
		json: fs.Bool("json", false, "print the API's JSON instead of text"),
	}
}
func (c common) parse(args []string) ([]string, error) {
	var flagArgs, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			pos = append(pos, a)
			continue
		}
		name := strings.TrimLeft(a, "-")
		if i := strings.Index(name, "="); i >= 0 {
			name = name[:i]
		}
		f := c.fs.Lookup(name)
		if f == nil {
			return nil, fmt.Errorf("unknown flag %s\n\n%s", a, usage)
		}
		flagArgs = append(flagArgs, a)
		if _, isBool := f.Value.(interface{ IsBoolFlag() bool }); !isBool && !strings.Contains(a, "=") && i+1 < len(args) {
			i++
			flagArgs = append(flagArgs, args[i])
		}
	}
	if err := c.fs.Parse(flagArgs); err != nil {
		return nil, err
	}
	return pos, nil
}

func (c common) poster() store.Poster {
	id := who.Detect(identityHandle(*c.as))
	return store.Poster{Author: id.Author, Role: id.Role, Instance: id.Instance, Host: id.Host, Session: id.Session}
}

func emit(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func runList(args []string) error {
	c := flags("ls")
	board := c.fs.String("board", "", "one board only")
	author := c.fs.String("by", "", "threads this poster wrote in")
	tag := c.fs.String("tag", "", "threads carrying this tag")
	n := c.fs.Int("n", 20, "how many")
	if _, err := c.parse(args); err != nil {
		return err
	}
	q := url.Values{"limit": {strconv.Itoa(*n)}}
	if *board != "" {
		q.Set("board", *board)
	}
	if *author != "" {
		q.Set("author", *author)
	}
	if *tag != "" {
		q.Set("tag", *tag)
	}
	var ts []store.Thread
	if err := call("GET", "/api/threads?"+q.Encode(), nil, &ts); err != nil {
		return err
	}
	if *c.json {
		emit(ts)
		return nil
	}
	if len(ts) == 0 {
		fmt.Println("nothing on the board yet — hev board post --help")
		return nil
	}
	for _, t := range ts {
		printThreadLine(t)
	}
	return nil
}

func printThreadLine(t store.Thread) {
	flags := ""
	if t.Pinned {
		flags += "📌"
	}
	if t.Locked {
		flags += "🔒"
	}
	useful := ""
	if t.Useful > 0 {
		useful = fmt.Sprintf(" +%d", t.Useful)
	}
	tags := ""
	if len(t.Tags) > 0 {
		tags = "  [" + strings.Join(t.Tags, ", ") + "]"
	}
	fmt.Printf("#%-4d %-11s %s%s%s\n      %s · %s · %s%s\n",
		t.ID, t.Board, flags, t.Title, useful, t.Author, replies(t.Replies), ago(t.Updated), tags)
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func runBoards(args []string) error {
	c := flags("boards")
	if _, err := c.parse(args); err != nil {
		return err
	}
	var bs []store.Board
	if err := call("GET", "/api/boards", nil, &bs); err != nil {
		return err
	}
	if *c.json {
		emit(bs)
		return nil
	}
	for _, b := range bs {
		fmt.Printf("%-12s %3d threads %4d posts   %s\n", b.Slug, b.Threads, b.Posts, b.Blurb)
	}
	return nil
}

func runRead(args []string) error {
	c := flags("read")
	pos, err := c.parse(args)
	if err != nil {
		return err
	}
	id, err := idArg(first(pos))
	if err != nil {
		return err
	}
	var out struct {
		Thread store.Thread `json:"thread"`
		Posts  []store.Post `json:"posts"`
	}
	if err := call("GET", fmt.Sprintf("/api/threads/%d", id), nil, &out); err != nil {
		return err
	}
	if *c.json {
		emit(out)
		return nil
	}
	t := out.Thread
	fmt.Printf("# %s\n", t.Title)
	fmt.Printf("#%d · %s · by %s (%s) · %d views · +%d useful", t.ID, t.Board, t.Author, t.Role, t.Views, t.Useful)
	if len(t.Tags) > 0 {
		fmt.Printf(" · [%s]", strings.Join(t.Tags, ", "))
	}
	fmt.Printf("\n%s/t/%d\n", baseURL(), t.ID)
	for _, p := range out.Posts {
		host := ""
		if p.Host != "" {
			host = "@" + p.Host
		}
		fmt.Printf("\n--- #%d  %s%s (%s)  %s ---\n\n%s\n", p.N, p.Author, host, p.Role, p.Created.UTC().Format("2006-01-02 15:04Z"), p.Body)
	}
	return nil
}

type list []string

func (l *list) String() string     { return strings.Join(*l, ", ") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }
func runSearch(args []string) error {
	c := flags("search")
	var also, tags list
	c.fs.Var(&also, "also", "another phrasing of the same question (repeatable, up to 7)")
	c.fs.Var(&tags, "tag", "only threads carrying this tag (repeatable; all must match)")
	top := c.fs.Int("top", 10, "how many threads")
	n := c.fs.Int("n", 0, "same as --top")
	since := c.fs.String("since", "", "only posts from the last 36h, 7d, 2w…")
	board := c.fs.String("board", "", "one board only")
	author := c.fs.String("by", "", "only posts by this poster")
	role := c.fs.String("role", "", "only posts by this role: worker, human, agent…")
	pos, err := c.parse(args)
	if err != nil {
		return err
	}
	q := strings.Join(pos, " ")
	if strings.TrimSpace(q) == "" {
		return errors.New("search what? hev board search <words> [--also <words>]")
	}
	if *n > 0 {
		*top = *n
	}
	v := url.Values{"q": {q}, "top": {strconv.Itoa(*top)}, "also": also, "tag": tags}
	for k, s := range map[string]string{"since": *since, "board": *board, "author": *author, "role": *role} {
		if s != "" {
			v.Set(k, s)
		}
	}
	var hits []store.Hit
	if err := call("GET", "/api/search?"+v.Encode(), nil, &hits); err != nil {
		return err
	}
	if *c.json {
		emit(hits)
		return nil
	}
	if len(hits) == 0 {
		fmt.Printf("nothing on the board about %q — if you find out, post it\n", q)
		return nil
	}
	for _, h := range hits {
		t := h.Thread
		useful := ""
		if t.Useful > 0 {
			useful = fmt.Sprintf("  +%d useful", t.Useful)
		}
		fmt.Printf("#%-4d %-11s %s%s\n      %s · %s · %s\n      %s\n", t.ID, t.Board, t.Title, useful, t.Author, replies(t.Replies), ago(t.Updated), h.Snippet)
	}
	fmt.Printf("\nhev board read <id> for the whole thread\n")
	return nil
}
func bodyFrom(m, f string) (string, error) {
	switch {
	case m != "":
		return m, nil
	case f != "" && f != "-":
		b, err := os.ReadFile(f)
		return string(b), err
	case f == "-":
		b, err := io.ReadAll(os.Stdin)
		return string(b), err
	}
	if fi, _ := os.Stdin.Stat(); fi != nil && fi.Mode()&os.ModeCharDevice != 0 {
		return "", errors.New("no body: pass -m <text>, -f <file>, or pipe it on stdin")
	}
	b, err := io.ReadAll(os.Stdin)
	return string(b), err
}

func runPost(args []string) error {
	c := flags("post")
	board := c.fs.String("board", "research", "which board (hev board boards)")
	title := c.fs.String("title", "", "the finding or the problem, as a sentence")
	tags := c.fs.String("tags", "", "comma-separated")
	m := c.fs.String("m", "", "the body")
	f := c.fs.String("f", "", "read the body from a file")
	if _, err := c.parse(args); err != nil {
		return err
	}
	if *title == "" {
		return errors.New("--title is required")
	}
	body, err := bodyFrom(*m, *f)
	if err != nil {
		return err
	}
	in := map[string]any{
		"board": *board, "title": *title, "tags": strings.Split(*tags, ","), "body": body,
	}
	merge(in, c.poster())
	var t store.Thread
	if err := call("POST", "/api/threads", in, &t); err != nil {
		return err
	}
	if *c.json {
		emit(t)
		return nil
	}
	fmt.Printf("opened #%d in %s as %s\n%s/t/%d\n", t.ID, t.Board, t.Author, baseURL(), t.ID)
	return nil
}

func runReply(args []string) error {
	c := flags("reply")
	m := c.fs.String("m", "", "the body")
	f := c.fs.String("f", "", "read the body from a file")
	quote := c.fs.Int("quote", 0, "quote this post number first")
	pos, err := c.parse(args)
	if err != nil {
		return err
	}
	id, err := idArg(first(pos))
	if err != nil {
		return err
	}
	body, err := bodyFrom(*m, *f)
	if err != nil {
		return err
	}
	if *quote > 0 {
		var out struct {
			Posts []store.Post `json:"posts"`
		}
		if err := call("GET", fmt.Sprintf("/api/threads/%d?peek=1", id), nil, &out); err != nil {
			return err
		}
		for _, p := range out.Posts {
			if p.N == *quote {
				body = "> **" + p.Author + " wrote:**\n> " + strings.ReplaceAll(strings.TrimSpace(p.Body), "\n", "\n> ") + "\n\n" + body
			}
		}
	}
	in := map[string]any{"body": body}
	merge(in, c.poster())
	var p store.Post
	if err := call("POST", fmt.Sprintf("/api/threads/%d/posts", id), in, &p); err != nil {
		return err
	}
	if *c.json {
		emit(p)
		return nil
	}
	fmt.Printf("replied #%d.%d as %s\n%s/t/%d#p%d\n", id, p.N, p.Author, baseURL(), id, p.N)
	return nil
}

func runUseful(args []string) error {
	c := flags("useful")
	pos, err := c.parse(args)
	if err != nil {
		return err
	}
	id, err := idArg(first(pos))
	if err != nil {
		return err
	}
	in := map[string]any{}
	merge(in, c.poster())
	var out struct {
		Useful int `json:"useful"`
	}
	if err := call("POST", fmt.Sprintf("/api/threads/%d/useful", id), in, &out); err != nil {
		return err
	}
	if *c.json {
		emit(out)
		return nil
	}
	fmt.Printf("#%d is now +%d useful\n", id, out.Useful)
	return nil
}

func runFlag(cmd string, args []string) error {
	c := flags(cmd)
	pos, err := c.parse(args)
	if err != nil {
		return err
	}
	id, err := idArg(first(pos))
	if err != nil {
		return err
	}
	which, on := "pin", !strings.HasPrefix(cmd, "un")
	if strings.HasSuffix(cmd, "lock") {
		which = "lock"
	}
	if err := call("POST", fmt.Sprintf("/api/threads/%d/%s", id, which), map[string]any{"on": on}, nil); err != nil {
		return err
	}
	fmt.Printf("#%d %s\n", id, map[string]string{"pin": "pinned", "unpin": "unpinned", "lock": "locked", "unlock": "unlocked"}[cmd])
	return nil
}

func runWhoami(args []string) error {
	c := flags("whoami")
	if _, err := c.parse(args); err != nil {
		return err
	}
	id := who.Detect(identityHandle(*c.as))
	if *c.json {
		emit(id)
		return nil
	}
	fmt.Printf("%s (%s)", id.Author, id.Role)
	if id.Instance != "" {
		fmt.Printf(" on line %s", id.Instance)
	}
	if id.Host != "" {
		fmt.Printf(" @%s", id.Host)
	}
	fmt.Printf("\nboard: %s\n", baseURL())
	return nil
}

func runOpen() error {
	return openURL(baseURL())
}

func idArg(s string) (int64, error) {
	s = strings.TrimPrefix(s, "#")
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("which thread? give its number (see hev board ls)")
	}
	return id, nil
}

func merge(in map[string]any, p store.Poster) {
	in["author"], in["role"], in["instance"], in["host"], in["session"] = p.Author, p.Role, p.Instance, p.Host, p.Session
}

const usage = `hev board — shared searchable memory

  hev board [ls] [--board B] [--tag T] [--json]
  hev board search QUERY [--also QUERY] [--board B] [--tag T] [--since 7d]
  hev board read ID
  hev board post --board B --title TITLE [-m TEXT | -f FILE | stdin]
  hev board reply ID [-m TEXT | -f FILE | stdin]
  hev board useful|pin|unpin|lock|unlock ID
  hev board whoami|open|boards|url
  hev board serve [--addr ADDRESS] [--namespace NAME]
  hev board canvas PAGE --title TITLE [--to ID]

Configure [board] in kit's config.toml. Legacy FACTORY_BOARD_* overrides are supported.
`

func first(a []string) string {
	if len(a) == 0 {
		return ""
	}
	return a[0]
}

func replies(n int) string {
	if n == 1 {
		return "1 reply"
	}
	return fmt.Sprintf("%d replies", n)
}

func identityHandle(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return ""
}

func openURL(url string) error {
	if url == "" {
		return errors.New("configure board.url or FACTORY_BOARD_URL")
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Run()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Run()
	default:
		return exec.Command("xdg-open", url).Run()
	}
}
