package serve

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/hev/kit/internal/trace"
)

func TestWithLinksAddsWrapperPagesToTheTopBar(t *testing.T) {
	w := httptest.NewRecorder()
	New(&fakeReader{}).WithLinks(Link{Label: "Team", Href: "/team"}, Link{Label: "<x>", Href: "javascript:alert(1)"}).
		Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	body := w.Body.String()
	if !strings.Contains(body, `<a href="/team">Team</a>`) {
		t.Fatal("team link missing from the top bar")
	}
	if strings.Contains(body, "javascript:") || strings.Contains(body, "<!--__LINKS__-->") {
		t.Fatal("unsafe link rendered or placeholder left behind")
	}
}

func TestAuthorIsAFacetAndAFilter(t *testing.T) {
	rows := []trace.SessionRow{{SessionID: "a", Author: "Ada"}, {SessionID: "b", Author: "Grace"}, {SessionID: "c"}}
	if got := facets(rows)["author"]; !reflect.DeepEqual(got, []string{"Ada", "Grace"}) {
		t.Fatalf("author facet = %v", got)
	}
	filter, _, err := sessionFilter(url.Values{"author": {"Ada"}}, rows)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fmt.Sprint(filter), "[author In [Ada]]") {
		t.Fatalf("filter lacks the author clause: %v", filter)
	}
}
