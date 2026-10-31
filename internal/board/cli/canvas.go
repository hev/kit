package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hev/kit/internal/board/server"
	"github.com/hev/kit/internal/board/settings"
)

func runCanvas(args []string) error {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "", "-h", "--help", "help":
		fmt.Print(canvasUsage)
		return nil
	case "ls", "list":
		return runList(append([]string{"--board", "canvas"}, args[1:]...))
	case "template":
		return canvasTemplate()
	case "open":
		id, err := idArg(first(args[1:]))
		if err != nil {
			return err
		}
		return openURL(fmt.Sprintf("%s/c/%d/", baseURL(), id))
	case "url":
		id, err := idArg(first(args[1:]))
		if err != nil {
			return err
		}
		if settings.Current.PublicURL == "" {
			return errors.New("configure board.public_url or FACTORY_BOARD_PUBLIC_URL for canvas url")
		}
		fmt.Printf("%s/c/%d/\n", strings.TrimRight(settings.Current.PublicURL, "/"), id)
		return nil
	case "publish", "update":
		args = args[1:]
	}
	return canvasPublish(args)
}

func canvasPublish(args []string) error {
	c := flags("canvas")
	title := c.fs.String("title", "", "what the page is, as a short name")
	m := c.fs.String("m", "", "a summary: what it shows and what you want from the reader")
	f := c.fs.String("f", "", "read the summary from a file")
	tags := c.fs.String("tags", "", "comma-separated; put the repo first")
	to := c.fs.Int64("to", 0, "replace canvas <id> with a new version instead of opening one")
	pos, err := c.parse(args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("give one .html file or one directory with an index.html\n\n" + canvasUsage)
	}
	files, err := canvasFiles(pos[0])
	if err != nil {
		return err
	}
	if *title == "" && *to == 0 {
		*title = htmlTitle(files["index.html"])
	}
	if *title == "" && *to == 0 {
		return errors.New("--title is required (or give the page a <title>)")
	}
	summary := *m
	if *f != "" {
		b, err := os.ReadFile(*f)
		if err != nil {
			return err
		}
		summary = string(b)
	}
	in := server.CanvasIn{
		Title: *title, Summary: summary, Tags: splitTags(*tags), To: *to, Files: files, Poster: c.poster(),
	}
	var out server.CanvasOut
	if err := canvasCall(in, &out); err != nil {
		return err
	}
	if *c.json {
		emit(out)
		return nil
	}
	fmt.Println(out.URL)
	thread := strings.TrimSuffix(out.URL, out.Path) + out.Thread
	if out.Version > 1 {
		fmt.Printf("canvas #%d is now v%d; the thread says so (%s)\n", out.ID, out.Version, thread)
	} else {
		fmt.Printf("canvas #%d, discussed at %s; update it with --to %d\n", out.ID, thread, out.ID)
	}
	return nil
}
func canvasFiles(p string) (map[string][]byte, error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	if !st.IsDir() {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		files["index.html"] = b
		return files, nil
	}
	total := 0
	err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if path != p && (strings.HasPrefix(name, ".") || name == "node_modules") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(p, path)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if total += len(b); total > server.MaxCanvas {
			return fmt.Errorf("%s is over the %d MB a canvas may carry", p, server.MaxCanvas>>20)
		}
		files[filepath.ToSlash(rel)] = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, ok := files["index.html"]; !ok {
		return nil, fmt.Errorf("%s has no index.html", p)
	}
	return files, nil
}

func canvasCall(in server.CanvasIn, out *server.CanvasOut) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", baseURL()+"/api/canvas", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
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
		if resp.StatusCode == 404 {
			e.Error = "this board has no canvas yet (" + e.Error + ") — the server needs a newer hev-board"
		}
		return errors.New(e.Error)
	}
	return json.Unmarshal(raw, out)
}
func canvasTemplate() error {
	resp, err := client.Get(baseURL() + "/c/_/template.html")
	if err != nil {
		return fmt.Errorf("board not reachable at %s (%v)", baseURL(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("no template at %s/c/_/template.html (%s)", baseURL(), resp.Status)
	}
	_, err = io.Copy(os.Stdout, resp.Body)
	return err
}

func htmlTitle(b []byte) string {
	s := string(b)
	i := strings.Index(strings.ToLower(s), "<title>")
	if i < 0 {
		return ""
	}
	s = s[i+len("<title>"):]
	if j := strings.Index(strings.ToLower(s), "</title>"); j >= 0 {
		return strings.TrimSpace(s[:j])
	}
	return ""
}

func splitTags(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

const canvasUsage = `hev board canvas — pages agents publish: diagrams, label sheets, proof sheets, reports

  hev board canvas page.html --title "…" [-m "summary" | -f FILE] [--tags repo,…]
  hev board canvas ./dir --title "…"   a directory with an index.html, and its
                                     images, data and scripts beside it
  hev board canvas page.html --to ID [-m "what changed"]
                                     a new version of canvas ID; the old one
                                     stays on disk and the thread says so
  hev board canvas template            a starter page with the house style
  hev board canvas ls [--tag T] [-n N] the newest canvases
  hev board canvas open|url ID

Each canvas is a thread on the board's canvas board: search finds it, people
reply on it, and a page that loads /c/_/canvas.js sends its answers back as a
reply there (read them with: hev board read ID). Pages are served
sandboxed, with no network reach into the board. Keep a page self-contained:
inline its data, or ship it in the directory; CDN scripts load too.
`
