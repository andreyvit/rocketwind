package rocketwind

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestCompilerEdgesAgainstUpstream(t *testing.T) {
	data, err := os.ReadFile("testdata/upstream/compiler-edges.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		CSS, LoadError, Result, Error string
		Sheets                        map[string]string
		Classes                       []string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 37 {
		t.Fatalf("got %d compiler cases", len(cases))
	}
	for _, c := range cases {
		config := Config{}
		if c.Sheets != nil || c.LoadError != "" {
			config.LoadStylesheet = func(id, base string) (Stylesheet, error) {
				if c.LoadError != "" {
					return Stylesheet{}, errors.New(c.LoadError)
				}
				return Stylesheet{Content: c.Sheets[id], Path: id}, nil
			}
		}
		compiler, err := Compile(c.CSS, config)
		var got string
		if err == nil {
			got, err = compiler.Build(c.Classes)
		}
		if c.Error != "" {
			if err == nil || err.Error() != c.Error {
				t.Errorf("%s: got %v; want %s", c.CSS, err, c.Error)
			}
		} else if err != nil || got != strings.TrimPrefix(c.Result, "/*! tailwindcss v4.3.4 | MIT License | https://tailwindcss.com */\n") {
			t.Errorf("%s: %v\ngot %s\nwant %s", c.CSS, err, got, c.Result)
		}
	}
}
