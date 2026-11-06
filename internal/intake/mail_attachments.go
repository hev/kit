package intake

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const mailAttachmentCap int64 = 26214400

type MailAttachment struct {
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Bytes   int64  `json:"bytes"`
	SHA256  string `json:"sha256"`
	Stored  bool   `json:"stored"`
	Skipped string `json:"skipped"`
}
type mailAsset struct {
	index     int
	name, url string
}

const mailUploadMutation = `mutation IntakeMailUpload($type: String!, $name: String!, $size: Int!) { fileUpload(contentType: $type, filename: $name, size: $size) { success uploadFile { uploadUrl assetUrl headers { key value } } } }`
const mailAttachMutation = `mutation IntakeMailAttach($input: AttachmentCreateInput!) { attachmentCreate(input: $input) { success } }`

// Never follow a redirect with either source credentials or signed headers.
func (p *Poller) attachmentHTTP(req *http.Request) (*http.Response, error) {
	c := *p.client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c.Do(req)
}
func attachmentText(s string) string {
	s = strings.Join(strings.Fields(cleanMail(s, 300)), " ")
	return strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]", "*", "\\*", "`", "\\`", "<", "&lt;", ">", "&gt;", "!", "\\!").Replace(s)
}
func assetURL(s string) bool {
	u, e := url.Parse(s)
	return e == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && !strings.ContainsAny(s, "\r\n<>")
}
func (p *Poller) uploadMailAttachment(ctx context.Context, m MailItem, a MailAttachment) (string, string, bool) {
	u, e := url.Parse(p.cfg.Email.URL)
	if e != nil {
		return "", "invalid mailbox URL", false
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + m.ID + "/attachments/" + strconv.Itoa(a.Index)
	base, _ := url.Parse(p.cfg.Email.URL)
	u.RawPath = strings.TrimRight(base.EscapedPath(), "/") + "/" + url.PathEscape(m.ID) + "/attachments/" + strconv.Itoa(a.Index)
	u.RawQuery = ""
	req, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if e != nil {
		return "", "invalid download request", false
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv(p.cfg.Email.TokenEnv))
	res, e := p.attachmentHTTP(req)
	if e != nil {
		return "", "download transport failed", true
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Sprintf("download HTTP %d", res.StatusCode), res.StatusCode >= 500
	}
	data, e := io.ReadAll(io.LimitReader(res.Body, a.Bytes+1))
	if e != nil {
		return "", "download read failed", true
	}
	if int64(len(data)) != a.Bytes {
		return "", "size mismatch", false
	}
	h := sha256.Sum256(data)
	hash := hex.EncodeToString(h[:])
	if !strings.EqualFold(hash, a.SHA256) || !strings.EqualFold(hash, res.Header.Get("X-Content-SHA256")) {
		return "", "hash mismatch", false
	}
	var d struct {
		FileUpload struct {
			Success    bool
			UploadFile struct {
				UploadURL, AssetURL string
				Headers             []struct{ Key, Value string }
			}
		}
	}
	e = p.gqlRequest(ctx, mailUploadMutation, map[string]any{"type": a.Type, "name": strings.Join(strings.Fields(cleanMail(a.Name, 300)), " "), "size": a.Bytes}, &d, true)
	if e != nil {
		var status linearHTTPError
		return "", "Linear upload request failed", (errors.As(e, &status) && status >= 500) || (!errors.As(e, &status) && !errors.Is(e, errGraphQL))
	}
	f := d.FileUpload.UploadFile
	if !d.FileUpload.Success || !assetURL(f.UploadURL) || !assetURL(f.AssetURL) {
		return "", "invalid Linear upload result", false
	}
	req, e = http.NewRequestWithContext(ctx, "PUT", f.UploadURL, bytes.NewReader(data))
	if e != nil {
		return "", "invalid upload request", false
	}
	req.Header.Set("Content-Type", a.Type)
	for _, h := range f.Headers {
		req.Header.Set(h.Key, h.Value)
	}
	res2, e := p.attachmentHTTP(req)
	if e != nil {
		return "", "upload transport failed", true
	}
	defer res2.Body.Close()
	if res2.StatusCode < 200 || res2.StatusCode >= 300 {
		return "", fmt.Sprintf("upload HTTP %d", res2.StatusCode), res2.StatusCode >= 500
	}
	return f.AssetURL, "", false
}
func (p *Poller) prepareMailAttachments(ctx context.Context, m MailItem) (string, []mailAsset, bool, error) {
	if len(m.Attachments) == 0 {
		return "", nil, false, nil
	}
	limit := p.cfg.Email.MaxAttachmentBytes
	if limit <= 0 || limit > mailAttachmentCap {
		limit = mailAttachmentCap
	}
	var total int64
	description := "\n\n### Attachments (untrusted)\n"
	var assets []mailAsset
	retry := false
	for _, a := range m.Attachments {
		name := attachmentText(a.Name)
		reason := ""
		asset := ""
		transient := false
		switch {
		case !a.Stored:
			reason = "not stored by mailbox"
			if a.Skipped != "" {
				reason += ": " + attachmentText(a.Skipped)
			}
		case a.Bytes < 0 || a.Bytes > limit-total:
			reason = "attachment size limit exceeded"
		default:
			total += a.Bytes
			asset, reason, transient = p.uploadMailAttachment(ctx, m, a)
		}
		if reason != "" {
			description += fmt.Sprintf("- %s: %s\n", name, reason)
			if e := p.sourceLog(Decision{Action: "triage", Decision: "attachment-failed"}, "email", fmt.Sprintf("attachment index %d: %s", a.Index, strings.Split(reason, ":")[0])); e != nil {
				return "", nil, false, e
			}
		} else {
			link := "<" + strings.ReplaceAll(strings.ReplaceAll(asset, "(", "%28"), ")", "%29") + ">"
			description += fmt.Sprintf("- [%s](%s) (%s, %d bytes)\n", name, link, attachmentText(a.Type), a.Bytes)
			if strings.HasPrefix(strings.ToLower(a.Type), "image/") {
				description += fmt.Sprintf("\n![%s](%s)\n\n", name, link)
			}
			assets = append(assets, mailAsset{a.Index, strings.Join(strings.Fields(cleanMail(a.Name, 300)), " "), asset})
		}
		retry = retry || transient
	}
	return description, assets, retry, nil
}
func (p *Poller) attachMailAssets(ctx context.Context, issue string, assets []mailAsset) {
	for _, a := range assets {
		var d struct{ AttachmentCreate struct{ Success bool } }
		if e := p.gqlRequest(ctx, mailAttachMutation, map[string]any{"input": map[string]any{"issueId": issue, "title": a.name, "url": a.url}}, &d, true); e != nil || !d.AttachmentCreate.Success {
			_ = p.sourceLog(Decision{Action: "triage", Decision: "attachment-failed"}, "email", fmt.Sprintf("attachment index %d: issue attachment failed", a.index))
		}
	}
}
