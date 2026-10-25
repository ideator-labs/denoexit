package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ideator-labs/denoexit/internal/shim"
)

const compatDate = "2026-10-01"

// lockedBuf collects a child's stderr so each request can see what the runtime logged.
type lockedBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type server struct {
	cmd  *exec.Cmd
	log  *lockedBuf
	base string
	done chan struct{}
}

func check(dir string, spec Spec) (Report, error) {
	rep := Report{App: filepath.Base(dir), SecondTarget: "workerd"}
	apis, err := scan(dir)
	if err != nil {
		return rep, err
	}
	rep.APIs = apis

	work, err := os.MkdirTemp("", "denoexit-")
	if err != nil {
		return rep, err
	}
	defer os.RemoveAll(work)

	deno, err := startDeno(dir, spec)
	if err != nil {
		return rep, err
	}
	defer deno.stop()
	rep.DenoBoot = deno.waitReady()

	var second *server
	if os.Getenv("DENOEXIT_BREAK") == "1" {
		rep.SecondTarget = "deno (DENOEXIT_BREAK=1, workerd skipped)"
		second, err = startDeno(dir, spec)
	} else {
		second, err = startWorkerd(dir, work, spec)
	}
	if err != nil {
		return rep, err
	}
	defer second.stop()
	rep.WorkerdBoot = second.waitReady()

	for _, r := range spec.Requests {
		res := Result{Name: r.Name}
		res.Deno = send(deno, r, rep.DenoBoot)
		res.Workerd = send(second, r, rep.WorkerdBoot)
		res.Same = rep.DenoBoot.OK && rep.WorkerdBoot.OK && res.Deno.Status == res.Workerd.Status && res.Deno.Body == res.Workerd.Body
		rep.Results = append(rep.Results, res)
	}
	rep.OK = rep.DenoBoot.OK && rep.WorkerdBoot.OK
	for _, r := range rep.Results {
		rep.OK = rep.OK && r.Same
	}
	return rep, nil
}

func startDeno(dir string, spec Spec) (*server, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("deno", "run", "--allow-net", "--allow-env", "--allow-read", spec.Entry)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), fmt.Sprintf("DENO_SERVE_ADDRESS=tcp:127.0.0.1:%d", port), "NO_COLOR=1")
	for k, v := range spec.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	return launch(cmd, port)
}

func startWorkerd(dir, work string, spec Spec) (*server, error) {
	bundle := filepath.Join(work, "app.js")
	out, err := exec.Command("deno", "bundle", "--quiet", "-o", bundle, filepath.Join(dir, spec.Entry)).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("deno bundle failed: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(work, "shim.js"), shim.ShimJS, 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(work, "entry.js"), shim.EntryJS, 0o644); err != nil {
		return nil, err
	}
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	conf := filepath.Join(work, "config.capnp")
	if err := os.WriteFile(conf, []byte(capnp(port, spec.Env)), 0o644); err != nil {
		return nil, err
	}
	bin, err := workerdBin()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, "serve", conf)
	cmd.Dir = work
	return launch(cmd, port)
}

func capnp(port int, env map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "using Workerd = import \"/workerd/workerd.capnp\";\n")
	fmt.Fprintf(&b, "const config :Workerd.Config = (\n  services = [(name = \"main\", worker = .w)],\n")
	fmt.Fprintf(&b, "  sockets = [(name = \"http\", address = \"127.0.0.1:%d\", http = (), service = \"main\")],\n);\n", port)
	fmt.Fprintf(&b, "const w :Workerd.Worker = (\n  modules = [\n")
	for _, m := range []string{"entry.js", "shim.js", "app.js"} {
		fmt.Fprintf(&b, "    (name = %q, esModule = embed %q),\n", m, m)
	}
	fmt.Fprintf(&b, "  ],\n  compatibilityDate = %q,\n  bindings = [\n", compatDate)
	for k, v := range env {
		fmt.Fprintf(&b, "    (name = %q, text = %q),\n", k, v)
	}
	fmt.Fprintf(&b, "  ],\n);\n")
	return b.String()
}

func workerdBin() (string, error) {
	if p := os.Getenv("DENOEXIT_WORKERD"); p != "" {
		return p, nil
	}
	if p, err := exec.LookPath("workerd"); err == nil {
		return p, nil
	}
	if m, _ := filepath.Glob("node_modules/@cloudflare/workerd-*/bin/workerd"); len(m) > 0 {
		return filepath.Abs(m[0])
	}
	return "", fmt.Errorf("workerd not found: run npm ci or set DENOEXIT_WORKERD")
}

func launch(cmd *exec.Cmd, port int) (*server, error) {
	s := &server{cmd: cmd, log: &lockedBuf{}, base: fmt.Sprintf("http://127.0.0.1:%d", port), done: make(chan struct{})}
	cmd.Stdout = s.log
	cmd.Stderr = s.log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() { cmd.Wait(); close(s.done) }()
	return s, nil
}

func (s *server) waitReady() Boot {
	addr := strings.TrimPrefix(s.base, "http://")
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-s.done:
			return Boot{OK: false, Error: firstError(s.log.String())}
		default:
		}
		if c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
			c.Close()
			return Boot{OK: true}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return Boot{OK: false, Error: "not listening after 15s"}
}

func (s *server) stop() {
	if s.cmd.Process != nil {
		syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
		<-s.done
	}
}

func send(s *server, r Request, boot Boot) Reply {
	if !boot.OK {
		return Reply{Error: "did not boot"}
	}
	before := len(s.log.String())
	req, err := http.NewRequest(r.Method, s.base+r.Path, strings.NewReader(r.Body))
	if err != nil {
		return Reply{Error: err.Error()}
	}
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return Reply{Error: err.Error()}
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	time.Sleep(150 * time.Millisecond)
	rep := Reply{Status: resp.StatusCode, Body: string(body)}
	if logs := s.log.String(); len(logs) > before {
		rep.Error = firstError(logs[before:])
	}
	return rep
}

var jsgError = regexp.MustCompile(`(?:jsg\.|Uncaught )((?:[A-Za-z]*Error): .*)`)

// firstError pulls the JavaScript error out of a runtime log chunk.
func firstError(log string) string {
	for _, line := range strings.Split(log, "\n") {
		if m := jsgError.FindStringSubmatch(line); m != nil {
			return strings.TrimSpace(m[1])
		}
	}
	return strings.TrimSpace(log)
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
