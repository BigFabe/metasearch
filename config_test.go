package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConfig(t *testing.T) {
	if _, err := loadConfig("config.example.json"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*config)
	}{
		{"missing origin", func(c *config) { c.PublicURL = "" }},
		{"origin path", func(c *config) { c.PublicURL = "https://example.com/path" }},
		{"origin credentials", func(c *config) { c.PublicURL = "https://user:secret@example.com" }},
		{"origin query", func(c *config) { c.PublicURL = "https://example.com/?secret=x" }},
		{"no placeholder", func(c *config) { c.DefaultSearch = "https://google.com/" }},
		{"relative", func(c *config) { c.DefaultSearch = "/search?q={query}" }},
		{"unsafe scheme", func(c *config) { c.DefaultSearch = "javascript:alert({query})" }},
		{"path placeholder", func(c *config) { c.DefaultSearch = "https://example.com/{query}?q=hello" }},
		{"fragment placeholder", func(c *config) { c.DefaultSearch = "https://example.com/?q=x#{query}" }},
		{"key placeholder", func(c *config) { c.DefaultSearch = "https://example.com/?{query}=x" }},
		{"extra placeholder", func(c *config) { c.DefaultSearch = "https://example.com/{query}?q={query}" }},
		{"invalid escape", func(c *config) { c.DefaultSearch = "https://example.com/?q={query}&x=%zz" }},
		{"newline", func(c *config) { c.DefaultSearch = "https://example.com/?q={query}\n" }},
		{"invalid bang", func(c *config) { c.Bangs = map[string]string{"!gh": "https://example.com/?q={query}"} }},
		{"invalid target", func(c *config) { c.Bangs = map[string]string{"gh": "https://example.com/"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testConfig()
			tc.change(&c)
			raw, _ := json.Marshal(c)
			if _, err := decodeConfig(strings.NewReader(string(raw))); err == nil {
				t.Fatal("accepted invalid config")
			}
		})
	}
	for _, raw := range []string{"{}", "null", "{broken", `{"secret-unknown-field":"sensitive"}`, `{} {}`} {
		if _, err := decodeConfig(strings.NewReader(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	c := testConfig()
	c.PublicURL += "/"
	raw, _ := json.Marshal(c)
	parsed, err := decodeConfig(strings.NewReader(string(raw)))
	if err != nil || parsed.PublicURL != "https://search.example.com" {
		t.Fatal("origin normalization failed")
	}
}

func TestConfigLimitsAndPrivateErrors(t *testing.T) {
	c := testConfig()
	raw, _ := json.Marshal(c)
	if _, err := decodeConfig(strings.NewReader(string(raw) + strings.Repeat(" ", 1<<20))); err == nil {
		t.Fatal("accepted oversized configuration")
	}
	c.PublicURL = "https://example.com/#"
	raw, _ = json.Marshal(c)
	if _, err := decodeConfig(strings.NewReader(string(raw))); err == nil {
		t.Fatal("accepted empty fragment")
	}
	for _, raw := range []string{`{"PRIVATE-CONFIG-SECRET":true}`, `{"public_url":"PRIVATE-CONFIG-SECRET"}`} {
		_, err := decodeConfig(strings.NewReader(raw))
		if err == nil || strings.Contains(err.Error(), "PRIVATE-CONFIG-SECRET") {
			t.Fatal("configuration error leaked input")
		}
	}
}

func TestBangsWithoutExclamationConfig(t *testing.T) {
	for _, value := range []string{"true", "false", ""} {
		raw := `{"public_url":"https://search.example.com","default_search":"https://www.google.com/search?q={query}"`
		if value != "" {
			raw += `,"bangs_without_exclamation":` + value
		}
		c, err := decodeConfig(strings.NewReader(raw + "}"))
		if err != nil {
			t.Fatal(err)
		}
		if c.BangsWithoutExclamation != (value == "true") {
			t.Fatalf("unexpected option value for %q", value)
		}
	}
}
