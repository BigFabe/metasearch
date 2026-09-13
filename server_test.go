package main

import (
	"bytes"
	"encoding/xml"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testConfig() config {
	return config{PublicURL: "https://search.example.com", DefaultSearch: "https://www.google.com/search?q={query}", Bangs: map[string]string{"gh": "https://github.com/search?q={query}", "w": "https://de.wikipedia.org/w/index.php?search={query}"}}
}

func TestResolve(t *testing.T) {
	c := testConfig()
	for _, tc := range []struct{ query, host, key, value string }{
		{"normal  text", "www.google.com", "q", "normal  text"},
		{"!gh linux", "github.com", "q", "linux"},
		{"linux !gh", "github.com", "q", "linux"},
		{"hello !gh world", "github.com", "q", "hello  world"},
		{"!unknown linux", "www.google.com", "q", "!unknown linux"},
		{"!GH linux", "www.google.com", "q", "!GH linux"},
		{"hello!gh linux", "www.google.com", "q", "hello!gh linux"},
		{"!gh, linux", "www.google.com", "q", "!gh, linux"},
		{"!w !gh linux", "de.wikipedia.org", "search", "!gh linux"},
		{"!unknown !gh linux !gh", "github.com", "q", "!unknown  linux !gh"},
		{"!gh", "github.com", "q", ""},
		{"\t!gh\u2003Grüße 日本語 &#+%\n", "github.com", "q", "Grüße 日本語 &#+%"},
		{"!gh a&admin=true#fragment\r\nInjected: value", "github.com", "q", "a&admin=true#fragment\r\nInjected: value"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			u, err := url.Parse(c.resolve(tc.query))
			if err != nil {
				t.Fatal(err)
			}
			if u.Host != tc.host || u.Query().Get(tc.key) != tc.value || len(u.Query()) != 1 || u.Fragment != "" {
				t.Fatalf("unexpected target: %s", u)
			}
		})
	}
}

func TestHTTP(t *testing.T) {
	var logs bytes.Buffer
	h := newHandler(testConfig(), log.New(&logs, "", 0))
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/", 200}, {"GET", "/healthz", 200}, {"GET", "/opensearch.xml", 200},
		{"GET", "/search?q=hello", 302}, {"HEAD", "/search?q=hello", 302},
		{"GET", "/search", 400}, {"GET", "/search?q=", 400}, {"GET", "/search?q=%20%09", 400},
		{"GET", "/search?q=%zz", 400}, {"GET", "/search?q=a&q=b", 400},
		{"POST", "/search?q=hello", 405}, {"GET", "/absent", 404},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != tc.status {
				t.Fatalf("status %d", w.Code)
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" {
				t.Fatal("missing privacy headers")
			}
			if w.Header().Get("Set-Cookie") != "" {
				t.Fatal("unexpected cookie")
			}
			if tc.status == 302 && (w.Header().Get("Location") != "https://www.google.com/search?q=hello" || w.Body.Len() != 0) {
				t.Fatal("invalid redirect")
			}
			if tc.path == "/" && !strings.Contains(w.Header().Get("Link"), "/opensearch.xml") {
				t.Fatal("missing discovery")
			}
			if tc.path == "/opensearch.xml" {
				var doc openSearch
				if err := xml.Unmarshal(w.Body.Bytes(), &doc); err != nil {
					t.Fatal(err)
				}
				if doc.XMLName.Space != "http://a9.com/-/spec/opensearch/1.1/" || doc.URL.Template != "https://search.example.com/search?q={searchTerms}" {
					t.Fatalf("invalid XML: %s", w.Body)
				}
			}
		})
	}
	if logs.Len() != 0 {
		t.Fatalf("requests were logged: %s", &logs)
	}
}

func TestPrivacyIncludingFailures(t *testing.T) {
	const secret = "PRIVATE-QUERY-7efab2"
	var logs bytes.Buffer
	logger := log.New(&logs, "", 0)
	h := newHandler(testConfig(), logger)
	for _, path := range []string{"/search?q=" + secret, "/search?q=" + secret + "&q=again", "/" + secret} {
		req := httptest.NewRequest("GET", path, nil)
		req.RemoteAddr = "192.0.2.77:12345"
		req.Header.Set("User-Agent", secret)
		req.Header.Set("Authorization", "Bearer "+secret)
		req.Header.Set("Referer", "https://example.org/"+secret)
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	w := httptest.NewRecorder()
	privacyHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(secret) }), logger).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 500 || strings.Contains(w.Body.String(), secret) {
		t.Fatal("panic response leaked")
	}
	s := newServer("", h, logger)
	s.ErrorLog.Printf("remote=192.0.2.77 url=%s", secret)
	if got := logs.String(); got != "request processing failed\nHTTP server error\n" {
		t.Fatalf("unsafe logs: %q", got)
	}
}

func TestNetworkPrivacy(t *testing.T) {
	var logs bytes.Buffer
	logger := log.New(&logs, "", 0)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := newServer("", newHandler(testConfig(), logger), logger)
	done := make(chan error, 1)
	go func() { done <- s.Serve(listener) }()
	defer func() {
		s.Close()
		<-done
		if logs.Len() != 0 {
			t.Errorf("network requests were logged: %s", &logs)
		}
	}()
	client := &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Get("http://" + listener.Addr().String() + "/search?q=NETWORK-SECRET")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 302 {
		t.Fatal(res.Status)
	}
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(time.Second))
	io.WriteString(conn, "GET /search?q=NETWORK-SECRET HTTP/1.1\r\nHost: example.org\r\nInvalid Header: SECRET-METADATA\r\n\r\n")
	body, err := io.ReadAll(conn)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("400 Bad Request")) {
		t.Fatalf("expected parser rejection: %s", body)
	}
}

func BenchmarkResolve(b *testing.B) {
	c := testConfig()
	b.ReportAllocs()
	for b.Loop() {
		c.resolve("!gh Grüße & linux")
	}
}

type discardResponse struct{ h http.Header }

func (w discardResponse) Header() http.Header         { return w.h }
func (w discardResponse) Write(p []byte) (int, error) { return len(p), nil }
func (w discardResponse) WriteHeader(int)             {}
func BenchmarkHandler(b *testing.B) {
	h := newHandler(testConfig(), log.New(io.Discard, "", 0))
	req := httptest.NewRequest("GET", "/search?q=%21gh+linux", nil)
	w := discardResponse{make(http.Header)}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		h.ServeHTTP(w, req)
	}
}
