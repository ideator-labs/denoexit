package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var denoCall = regexp.MustCompile(`\bDeno\.([A-Za-z]+)`)

// What denoexit's shim provides on workerd, and where each missing API goes on Workers.
var apiTable = map[string]struct {
	shimmed bool
	hint    string
}{
	"serve":            {true, "becomes the Worker fetch handler; the second (info) argument is not passed"},
	"env":              {true, "reads text bindings; set them as vars or secrets in wrangler config"},
	"openKv":           {false, "Workers KV (no atomics) or a Durable Object with SQLite"},
	"cron":             {false, "a scheduled() handler plus triggers.crons"},
	"readTextFile":     {false, "inline at build time or serve via static assets"},
	"readTextFileSync": {false, "inline at build time or serve via static assets"},
	"readFile":         {false, "inline at build time or serve via static assets"},
	"upgradeWebSocket": {false, "new WebSocketPair()"},
	"resolveDns":       {false, "no DNS API on Workers"},
	"Command":          {false, "no subprocesses on Workers"},
}

func scan(dir string) ([]APIUse, error) {
	var uses []APIUse
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}
		ext := filepath.Ext(path)
		if d.IsDir() || (ext != ".ts" && ext != ".js" && ext != ".tsx" && ext != ".mjs") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		rel, _ := filepath.Rel(dir, path)
		sc := bufio.NewScanner(f)
		for line := 1; sc.Scan(); line++ {
			for _, m := range denoCall.FindAllStringSubmatch(sc.Text(), -1) {
				t, known := apiTable[m[1]]
				if !known && unicode.IsUpper(rune(m[1][0])) {
					continue // a type such as Deno.ServeHandlerInfo, gone after bundling
				}
				hint := t.hint
				if !known {
					hint = "not in denoexit's shim; check the Workers runtime APIs"
				}
				uses = append(uses, APIUse{API: "Deno." + m[1], Where: fmt.Sprintf("%s:%d", rel, line), Shimmed: t.shimmed, Hint: hint})
			}
		}
		return sc.Err()
	})
	return uses, err
}
