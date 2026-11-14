package peersview_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestReviewExtraHidden(t *testing.T) {
	for i, v := range []string{`%20%2Fhome%2Fdelta-private.txt%20`, `+%2Fhome%2Fdelta-private.txt+`, `x=/home/delta-private.txt`, `label:/home/delta-private.txt`, `{"p":"/home/delta-private.txt"}`, `[/home/delta-private.txt]`, `%5C%5Chost%5Cdelta-private.txt`, `%20C%3A%5Cdelta-private.txt`, `FILE%3a%2f%2fhost%2fdelta-private.txt`, `log:/Users/x/delta-private.txt`, `at:C:\Users\delta-private.txt`, `see:~/delta-private.txt`, `%2520%252Fhome%252Fdelta-private.txt`, `%E2%80%83%2Fhome%2Fdelta-private.txt`, `%09C%3A%5Cdelta-private.txt`} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if n := strings.Count(deltaRender(t, v), "delta-private.txt"); n != 0 {
				t.Errorf("leaked %d contexts input=%s", n, v)
			}
		})
	}
}
func TestReviewExtraKept(t *testing.T) {
	for _, v := range []string{`https://x.test/?q=a%2Fb`, `This is C: drive`, `1:1`, `person@example.com`, `key:value`, `host:8080/path`, `https://x.test//home/delta-private.txt`, `file%25253Ahome%25252Fdelta-private.txt`} {
		if n := strings.Count(deltaRender(t, v), v); n != 18 {
			t.Errorf("input=%s kept=%d want=18", v, n)
		}
	}
}
func TestReviewMegabyte(t *testing.T) {
	v := strings.Repeat("x", 1024*1024) + "=/home/delta-private.txt"
	start := time.Now()
	b := deltaRender(t, v)
	t.Logf("1 MB rendered in %s", time.Since(start))
	if strings.Contains(b, "delta-private.txt") {
		t.Fatal("leak")
	}
}
