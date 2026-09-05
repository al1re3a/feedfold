package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestCanonical(t *testing.T) {
	got, err := canonical("https://EXAMPLE.com/a?utm_source=x&b=2#top")
	if err != nil || got != "https://example.com/a?b=2" {
		t.Fatal(got, err)
	}
}
func TestInvalidLinks(t *testing.T) {
	for _, raw := range []string{"relative", "file:///tmp/a", "javascript:bad", "https://u:p@example.com", "https://example.com/?bad=%"} {
		if _, err := canonical(raw); err == nil {
			t.Fatal(raw)
		}
	}
}
func TestFeedsAndDedup(t *testing.T) {
	var groups [][]Entry
	for _, path := range []string{"examples/rss.xml", "examples/atom.xml"} {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		e, n, err := readFeed(f)
		if err != nil || n != 0 {
			t.Fatal(err, n)
		}
		groups = append(groups, e)
	}
	merged := merge(groups...)
	if len(merged) != 3 || merged[0].Title != "Offline-first reports" || !strings.Contains(merged[1].Title, "updated") {
		t.Fatal(merged)
	}
}
func TestBadXML(t *testing.T) {
	for _, x := range []string{"", "<broken>", "<other/>", "<rss>&unknown;</rss>"} {
		if _, _, err := readFeed(strings.NewReader(x)); err == nil {
			t.Fatal(x)
		}
	}
}
func TestSize(t *testing.T) {
	if _, _, err := readFeed(strings.NewReader(strings.Repeat("x", 5*1024*1024+1))); err == nil {
		t.Fatal("size limit")
	}
}
func TestMissingLink(t *testing.T) {
	e, n, err := readFeed(strings.NewReader("<rss><channel><item><title>missing</title></item></channel></rss>"))
	if err != nil || len(e) != 0 || n != 1 {
		t.Fatal(e, n, err)
	}
}
func TestDate(t *testing.T) {
	if date("unknown") != "" || date("2026-01-01T12:00:00+02:00") != "2026-01-01T10:00:00Z" {
		t.Fatal("date parsing")
	}
}
func TestHTML(t *testing.T) {
	data, err := render([]Entry{{Title: "<img> & title", URL: "https://example.com", Source: "<source>"}})
	if err != nil || strings.Contains(string(data), "<img>") || !strings.Contains(string(data), "&lt;img&gt;") {
		t.Fatal(string(data), err)
	}
}
func TestCLI(t *testing.T) {
	var out, stderr bytes.Buffer
	if run([]string{"--format", "html", "examples/rss.xml"}, &out, &stderr) != 0 || !strings.Contains(out.String(), "<!doctype html>") {
		t.Fatal(stderr.String())
	}
}
func TestCLIErrors(t *testing.T) {
	for _, args := range [][]string{{}, {"--format", "bad", "x"}, {"absent.xml"}} {
		var out, stderr bytes.Buffer
		if run(args, &out, &stderr) != 2 || out.Len() != 0 {
			t.Fatal(args, out.String())
		}
	}
}
func TestStableTie(t *testing.T) {
	a := Entry{URL: "https://b"}
	b := Entry{URL: "https://a"}
	if merge([]Entry{a, b})[0].URL != b.URL {
		t.Fatal("tie order")
	}
}
