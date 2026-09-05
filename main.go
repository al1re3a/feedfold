// FeedFold merges locally saved RSS and Atom files. It never fetches URLs.
package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

//go:embed web/*
var assets embed.FS

type Entry struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Published string `json:"published"`
	Source    string `json:"source"`
}
type Item struct {
	Title string `xml:"title"`
	Link  string `xml:"link"`
	Date  string `xml:"pubDate"`
}
type AtomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}
type AtomEntry struct {
	Title     string     `xml:"title"`
	Links     []AtomLink `xml:"link"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
}
type Feed struct {
	XMLName xml.Name
	Title   string `xml:"title"`
	Channel struct {
		Title string `xml:"title"`
		Items []Item `xml:"item"`
	} `xml:"channel"`
	Entries []AtomEntry `xml:"entry"`
}

func canonical(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", errors.New("entry needs an absolute HTTP(S) URL without credentials")
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", err
	}
	for key := range q {
		if strings.HasPrefix(strings.ToLower(key), "utm_") {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func date(raw string) string {
	for _, layout := range []string{time.RFC3339Nano, time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822} {
		if d, err := time.Parse(layout, strings.TrimSpace(raw)); err == nil {
			return d.UTC().Format(time.RFC3339)
		}
	}
	return ""
}
func readFeed(r io.Reader) ([]Entry, int, error) {
	data, err := io.ReadAll(io.LimitReader(r, 5*1024*1024+1))
	if err != nil {
		return nil, 0, err
	}
	if len(data) > 5*1024*1024 {
		return nil, 0, errors.New("feed exceeds 5 MiB")
	}
	var feed Feed
	if err = xml.Unmarshal(data, &feed); err != nil {
		return nil, 0, err
	}
	var entries []Entry
	switch feed.XMLName.Local {
	case "rss":
		for _, item := range feed.Channel.Items {
			entries = append(entries, Entry{item.Title, item.Link, date(item.Date), feed.Channel.Title})
		}
	case "feed":
		for _, item := range feed.Entries {
			link := ""
			for _, candidate := range item.Links {
				if candidate.Rel == "" || candidate.Rel == "alternate" {
					link = candidate.Href
					break
				}
			}
			published := date(item.Published)
			if published == "" {
				published = date(item.Updated)
			}
			entries = append(entries, Entry{item.Title, link, published, feed.Title})
		}
	default:
		return nil, 0, errors.New("expected RSS 2.0 or Atom feed")
	}
	if len(entries) > 10000 {
		return nil, 0, errors.New("feed exceeds 10000 entries")
	}
	valid := make([]Entry, 0, len(entries))
	skipped := 0
	for _, entry := range entries {
		link, err := canonical(entry.URL)
		if err != nil {
			skipped++
			continue
		}
		entry.URL = link
		entry.Title = strings.TrimSpace(entry.Title)
		if entry.Title == "" {
			entry.Title = entry.URL
		}
		valid = append(valid, entry)
	}
	return valid, skipped, nil
}
func merge(groups ...[]Entry) []Entry {
	unique := map[string]Entry{}
	for _, group := range groups {
		for _, entry := range group {
			old, ok := unique[entry.URL]
			if !ok || entry.Published > old.Published {
				unique[entry.URL] = entry
			}
		}
	}
	result := make([]Entry, 0, len(unique))
	for _, entry := range unique {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Published == result[j].Published {
			return result[i].URL < result[j].URL
		}
		return result[i].Published > result[j].Published
	})
	return result
}
func render(entries []Entry) ([]byte, error) {
	html, err := assets.ReadFile("web/report.html")
	if err != nil {
		return nil, err
	}
	css, err := assets.ReadFile("web/report.css")
	if err != nil {
		return nil, err
	}
	js, err := assets.ReadFile("web/report.js")
	if err != nil {
		return nil, err
	}
	page, err := template.New("report").Parse(string(html))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	// Only embedded, version-controlled assets are trusted; feed values stay escaped.
	err = page.Execute(&out, struct {
		Entries []Entry
		CSS     template.CSS
		JS      template.JS
	}{entries, template.CSS(css), template.JS(js)})
	return out.Bytes(), err
}
func run(args []string, out, stderr io.Writer) int {
	flags := flag.NewFlagSet("feedfold", flag.ContinueOnError)
	flags.SetOutput(stderr)
	format := flags.String("format", "json", "json or html")
	if flags.Parse(args) != nil {
		return 2
	}
	if flags.NArg() == 0 || (*format != "json" && *format != "html") {
		fmt.Fprintln(stderr, "usage: feedfold --format json|html SAVED_FEED...")
		return 2
	}
	var all []Entry
	skipped := 0
	for _, path := range flags.Args() {
		f, err := os.Open(path)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		entries, n, err := readFeed(f)
		f.Close()
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", path, err)
			return 2
		}
		all = append(all, entries...)
		skipped += n
		if len(all)+skipped > 10000 {
			fmt.Fprintln(stderr, "total exceeds 10000 entries")
			return 2
		}
	}
	entries := merge(all)
	var data []byte
	var err error
	if *format == "html" {
		data, err = render(entries)
	} else {
		data, err = json.MarshalIndent(entries, "", "  ")
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if _, err = out.Write(append(data, '\n')); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	fmt.Fprintf(stderr, "%d unique entries; %d duplicate entries; %d entries skipped for invalid/missing links\n", len(entries), len(all)-len(entries), skipped)
	return 0
}
func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
