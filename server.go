package main

import (
	"encoding/xml"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

func (c config) resolve(query string) string {
	target := c.DefaultSearch
	start := -1
	for i, r := range query {
		if unicode.IsSpace(r) {
			if start >= 0 {
				if template, ok := c.bang(query[start:i]); ok {
					return expand(template, strings.TrimSpace(query[:start]+query[i:]))
				}
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		if template, ok := c.bang(query[start:]); ok {
			return expand(template, strings.TrimSpace(query[:start]))
		}
	}
	return expand(target, query)
}

func (c config) bang(word string) (string, bool) {
	if !strings.HasPrefix(word, "!") {
		return "", false
	}
	target, ok := c.Bangs[word[1:]]
	return target, ok
}

func expand(template, query string) string {
	return strings.ReplaceAll(template, placeholder, url.QueryEscape(query))
}

type openSearch struct {
	XMLName     xml.Name `xml:"OpenSearchDescription"`
	XMLNS       string   `xml:"xmlns,attr"`
	ShortName   string
	Description string
	Encoding    string    `xml:"InputEncoding"`
	URL         searchURL `xml:"Url"`
}

type searchURL struct {
	Type     string `xml:"type,attr"`
	Method   string `xml:"method,attr"`
	Template string `xml:"template,attr"`
}

func newHandler(c config, logger *log.Logger) http.Handler {
	description, _ := xml.Marshal(openSearch{
		XMLNS: "http://a9.com/-/spec/opensearch/1.1/", ShortName: "Metasearch",
		Description: "Search router with custom bangs", Encoding: "UTF-8",
		URL: searchURL{"text/html", "GET", c.PublicURL + "/search?q={searchTerms}"},
	})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Link", "<"+c.PublicURL+"/opensearch.xml>; rel=\"search\"; type=\"application/opensearchdescription+xml\"; title=\"Metasearch\"")
			io.WriteString(w, "Metasearch: configure your browser with "+c.PublicURL+"/search?q=%s\n")
		case "/search":
			values, err := url.ParseQuery(r.URL.RawQuery)
			queries := values["q"]
			if err != nil || len(queries) != 1 || strings.TrimSpace(queries[0]) == "" {
				http.Error(w, "provide one non-empty q parameter", http.StatusBadRequest)
				return
			}
			// No redirect response body: neither query nor destination needs reflecting.
			w.Header().Set("Location", c.resolve(queries[0]))
			w.WriteHeader(http.StatusFound)
		case "/opensearch.xml":
			w.Header().Set("Content-Type", "application/opensearchdescription+xml; charset=utf-8")
			io.WriteString(w, xml.Header)
			w.Write(description)
		case "/healthz":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			io.WriteString(w, "ok\n")
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
	return privacyHandler(h, logger)
}

func privacyHandler(next http.Handler, logger *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		defer func() {
			if recover() != nil {
				logger.Print("request processing failed")
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// net/http diagnostics can contain remote addresses and panic values. Discard
// their payload completely, including errors generated before handler dispatch.
type privateErrorWriter struct{ logger *log.Logger }

func (w privateErrorWriter) Write(p []byte) (int, error) {
	w.logger.Print("HTTP server error")
	return len(p), nil
}

func newServer(addr string, handler http.Handler, logger *log.Logger) *http.Server {
	return &http.Server{
		Addr: addr, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 16 << 10,
		ErrorLog:       log.New(privateErrorWriter{logger}, "", 0),
	}
}
