package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

type Request struct {
	Name    string            `json:"name"`
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Body    string            `json:"body,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type Spec struct {
	Entry    string            `json:"entry"`
	Env      map[string]string `json:"env"`
	Requests []Request         `json:"requests"`
}

type APIUse struct {
	API     string `json:"api"`
	Where   string `json:"where"`
	Shimmed bool   `json:"shimmed"`
	Hint    string `json:"hint"`
}

type Reply struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
	Error  string `json:"error,omitempty"`
}

type Result struct {
	Name    string `json:"name"`
	Deno    Reply  `json:"deno"`
	Workerd Reply  `json:"workerd"`
	Same    bool   `json:"same"`
}

type Boot struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type Report struct {
	App          string   `json:"app"`
	WorkerdBoot  Boot     `json:"workerd_boot"`
	DenoBoot     Boot     `json:"deno_boot"`
	APIs         []APIUse `json:"apis"`
	Results      []Result `json:"results"`
	OK           bool     `json:"ok"`
	SecondTarget string   `json:"second_target"`
}

func main() {
	jsonOut := flag.String("json", "", "write the report as JSON to this file")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: denoexit [--json report.json] <app-dir>")
		fmt.Fprintln(os.Stderr, "app-dir must contain denoexit.json (entry, env, requests)")
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	dir, err := filepath.Abs(flag.Arg(0))
	if err != nil {
		fatal(err)
	}
	spec, err := loadSpec(dir)
	if err != nil {
		fatal(err)
	}
	rep, err := check(dir, spec)
	if err != nil {
		fatal(err)
	}
	if *jsonOut != "" {
		b, _ := json.MarshalIndent(rep, "", "  ")
		if err := os.WriteFile(*jsonOut, append(b, '\n'), 0o644); err != nil {
			fatal(err)
		}
	}
	printReport(os.Stdout, rep)
	if !rep.OK {
		os.Exit(1)
	}
}

func loadSpec(dir string) (Spec, error) {
	var s Spec
	b, err := os.ReadFile(filepath.Join(dir, "denoexit.json"))
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("denoexit.json: %w", err)
	}
	if s.Entry == "" || len(s.Requests) == 0 {
		return s, fmt.Errorf("denoexit.json needs entry and at least one request")
	}
	for i := range s.Requests {
		if s.Requests[i].Method == "" {
			s.Requests[i].Method = "GET"
		}
		if s.Requests[i].Name == "" {
			s.Requests[i].Name = s.Requests[i].Method + " " + s.Requests[i].Path
		}
	}
	return s, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "denoexit:", err)
	os.Exit(2)
}
