package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"strings"
)

const placeholder = "{query}"

type config struct {
	PublicURL               string            `json:"public_url"`
	DefaultSearch           string            `json:"default_search"`
	BangsWithoutExclamation bool              `json:"bangs_without_exclamation"`
	Bangs                   map[string]string `json:"bangs"`
}

func loadConfig(path string) (config, error) {
	f, err := os.Open(path)
	if err != nil {
		return config{}, errors.New("cannot open configuration")
	}
	defer f.Close()
	return decodeConfig(f)
}

func decodeConfig(r io.Reader) (config, error) {
	var c config
	limited := &io.LimitedReader{R: r, N: (1 << 20) + 1}
	d := json.NewDecoder(limited)
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, errors.New("configuration must be valid JSON with known fields")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return c, errors.New("configuration must contain one JSON object")
	}
	if limited.N == 0 {
		return c, errors.New("configuration must not exceed 1 MiB")
	}
	u, err := url.Parse(c.PublicURL)
	if err != nil || !absoluteHTTP(u) || u.RawQuery != "" || u.ForceQuery || strings.Contains(c.PublicURL, "#") || (u.Path != "" && u.Path != "/") {
		return c, errors.New("public_url must be an HTTP(S) origin without path, query, credentials or fragment")
	}
	c.PublicURL = strings.TrimSuffix(c.PublicURL, "/")
	if !validTemplate(c.DefaultSearch) {
		return c, errors.New("default_search must be an HTTP(S) URL with {query} only in a query parameter value")
	}
	for bang, target := range c.Bangs {
		if bang == "" || strings.IndexFunc(bang, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-')
		}) >= 0 {
			return c, errors.New("bang keys must contain only ASCII letters, digits, underscores or hyphens, without !")
		}
		if !validTemplate(target) {
			return c, errors.New("bang targets must be HTTP(S) URLs with {query} only in a query parameter value")
		}
	}
	return c, nil
}

func absoluteHTTP(u *url.URL) bool {
	return u != nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && u.Opaque == ""
}

func validTemplate(s string) bool {
	if strings.ContainsAny(s, "\r\n\t ") || !strings.Contains(s, placeholder) {
		return false
	}
	u, err := url.Parse(s)
	if err != nil || !absoluteHTTP(u) {
		return false
	}
	// Count literal placeholders only in parameter values, never in the URL authority,
	// path, fragment or parameter names. Escaped placeholders are not supported.
	n := 0
	if _, err := url.ParseQuery(u.RawQuery); err != nil {
		return false
	}
	for _, part := range strings.Split(u.RawQuery, "&") {
		_, value, ok := strings.Cut(part, "=")
		if ok {
			n += strings.Count(value, placeholder)
		}
	}
	return n == strings.Count(s, placeholder)
}
