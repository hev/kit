package intake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type attachmentTransport func(*http.Request) (*http.Response, error)

func (f attachmentTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestMailAttachments(t *testing.T) {
	for _, scenario := range []string{"three-assets", "hash", "download-4xx", "upload-4xx", "mail-5xx-recover", "linear-5xx-recover", "three-failed-polls", "skipped", "total-limit", "uncertain", "attach-failure", "redirect"} {
		t.Run(scenario, func(t *testing.T) {
			f := sourceSetup(t)
			data := []byte("opaque attachment bytes")
			h := sha256.Sum256(data)
			m := f.mail()
			for i, typ := range []string{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "image/png", "image/jpeg"} {
				m.Attachments = append(m.Attachments, MailAttachment{Index: i, Name: fmt.Sprintf("secret-name-%d", i), Type: typ, Bytes: int64(len(data)), SHA256: hex.EncodeToString(h[:]), Stored: true})
			}
			switch scenario {
			case "hash":
				m.Attachments[0].SHA256 = strings.Repeat("0", 64)
			case "skipped":
				m.Attachments[0].Stored = false
				m.Attachments[0].Skipped = "too large"
			case "total-limit":
				f.p.cfg.Email.MaxAttachmentBytes = int64(len(data))
			case "uncertain":
				f.uncertain = true
			}
			f.items = []MailItem{m}
			uploads, polls := 0, 0
			original := http.DefaultTransport
			f.p.client.Transport = attachmentTransport(func(r *http.Request) (*http.Response, error) {
				respond := func(status int, body string, headers http.Header) (*http.Response, error) {
					return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: headers, Request: r}, nil
				}
				if strings.Contains(r.URL.Path, "/attachments/") {
					if r.Header.Get("Authorization") != "Bearer test-mail-token" {
						t.Error("download credentials")
					}
					status := 200
					if scenario == "download-4xx" {
						status = 404
					}
					if (scenario == "mail-5xx-recover" && polls == 1) || scenario == "three-failed-polls" {
						status = 503
					}
					if scenario == "redirect" {
						return respond(302, "", http.Header{"Location": []string{"https://evil.invalid/steal"}})
					}
					return respond(status, string(data), http.Header{"X-Content-Sha256": []string{hex.EncodeToString(h[:])}})
				}
				if r.URL.Host == "signed.invalid" {
					if r.Header.Get("Authorization") != "signed-header" || r.Header.Get("X-Upload") != "yes" {
						t.Error("signed headers")
					}
					b, _ := io.ReadAll(r.Body)
					if string(b) != string(data) {
						t.Error("bytes changed")
					}
					uploads++
					status := 200
					if scenario == "upload-4xx" {
						status = 403
					}
					return respond(status, "", http.Header{})
				}
				if r.URL.Host == "evil.invalid" {
					t.Fatal("followed redirect")
				}
				if r.URL.String() == f.p.endpoint {
					if r.Header.Get("Authorization") != "test-key" {
						t.Error("Linear key missing")
					}
					b, _ := io.ReadAll(r.Body)
					r.Body = io.NopCloser(strings.NewReader(string(b)))
					var q struct{ Query string }
					_ = json.Unmarshal(b, &q)
					if strings.Contains(q.Query, "IntakeMailUpload") {
						if scenario == "linear-5xx-recover" && polls == 1 {
							return respond(503, "", http.Header{})
						}
						return respond(200, `{"data":{"fileUpload":{"success":true,"uploadFile":{"uploadUrl":"https://signed.invalid/upload","assetUrl":"https://assets.linear.app/asset","headers":[{"key":"Authorization","value":"signed-header"},{"key":"X-Upload","value":"yes"}]}}}}`, http.Header{})
					}
					if strings.Contains(q.Query, "IntakeMailAttach") {
						if f.creates != 1 {
							t.Error("attached before create")
						}
						if scenario == "attach-failure" {
							return respond(503, "", http.Header{})
						}
						return respond(200, `{"data":{"attachmentCreate":{"success":true}}}`, http.Header{})
					}
					if strings.Contains(q.Query, "IntakeMailCreate") && scenario == "three-assets" && uploads != 3 {
						t.Error("create before uploads")
					}
				} else if r.Header.Get("Authorization") != "Bearer test-mail-token" {
					t.Error("mail token confinement")
				}
				mailURL, _ := url.Parse(f.p.cfg.Email.URL)
				linearURL, _ := url.Parse(f.p.endpoint)
				if r.URL.Host != mailURL.Host && r.URL.Host != linearURL.Host {
					t.Fatalf("unexpected network host: %s", r.URL.Host)
				}
				return original.RoundTrip(r)
			})
			count := 1
			if strings.Contains(scenario, "recover") {
				count = 2
			}
			if scenario == "three-failed-polls" {
				count = 3
			}
			for polls = 1; polls <= count; polls++ {
				e := f.p.poll(context.Background())
				deferred := polls < count
				if scenario == "uncertain" {
					if e == nil {
						t.Fatal("expected uncertain create")
					}
					break
				}
				if deferred {
					if e == nil || f.creates != 0 {
						t.Fatal("transient failure created issue")
					}
				} else if e != nil {
					t.Fatal(e)
				}
				f.clock = f.clock.Add(2 * time.Hour)
				if deferred {
					old := f.p
					restored, err := newPoller(old.cfg, "test-key", f.out)
					if err != nil {
						t.Fatal(err)
					}
					restored.client = old.client
					restored.endpoint = old.endpoint
					restored.command = old.command
					restored.now = old.now
					f.p = restored
				}
			}
			if scenario == "uncertain" {
				f.clock = f.clock.Add(2 * time.Hour)
				pollSource(t, f)
			}
			if f.creates != 1 {
				t.Fatalf("creates=%d", f.creates)
			}
			description := f.input["description"].(string)
			if !strings.Contains(description, "Attachments (untrusted)") {
				t.Fatal("missing list")
			}
			if scenario == "three-assets" {
				if strings.Count(description, "https://assets.linear.app/asset") != 5 {
					t.Fatal(description)
				}
			}
			if scenario == "hash" && !strings.Contains(description, "hash mismatch") {
				t.Fatal(description)
			}
			if scenario == "skipped" && !strings.Contains(description, "too large") {
				t.Fatal(description)
			}
			if scenario == "three-failed-polls" && !strings.Contains(description, "HTTP 503") {
				t.Fatal(description)
			}
			if strings.Contains(f.out.String(), "secret-name") || strings.Contains(f.out.String(), string(data)) {
				t.Fatal("attachment leaked to logs")
			}
			for _, order := range f.orders {
				if strings.Contains(order, "secret-name") {
					t.Fatal("name in order")
				}
			}
			pollSource(t, f)
			if f.creates != 1 {
				t.Fatal("duplicate issue")
			}
		})
	}
}
