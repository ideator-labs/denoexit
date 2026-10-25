#!/usr/bin/env bash
# Proves denoexit catches the ways a Deno Deploy app breaks on real workerd.
# Usage: bash demo.sh          normal run, expect 5/5 PASS
#        bash demo.sh --break  replace workerd with a second deno, expect failures
set -u
cd "$(dirname "$0")"

if [ "${1:-}" = "--break" ]; then
  export DENOEXIT_BREAK=1
  echo "BREAK MODE: the second runtime is deno, not workerd. Checks 2-4 should fail."
fi

for tool in go deno jq; do
  command -v "$tool" >/dev/null || { echo "missing prerequisite: $tool"; exit 2; }
done
[ -d node_modules/@cloudflare ] || npm ci --ignore-scripts --silent || { echo "npm ci failed"; exit 2; }

echo "go:      $(go version | awk '{print $3}')"
echo "deno:    $(deno --version | head -1)"
echo "workerd: $(node_modules/@cloudflare/workerd-*/bin/workerd --version)"

mkdir -p bin out
go build -o bin/denoexit . || { echo "build failed"; exit 2; }

run() {
  local app=$1
  ./bin/denoexit --json "out/$app.json" "examples/$app" > "out/$app.txt"
  echo $? > "out/$app.exit"
  echo; echo "----- denoexit examples/$app (exit $(cat out/$app.exit)) -----"
  cat "out/$app.txt"
}
run before
run halfway
run after

pass=0; fail=0
check() {
  local n=$1 desc=$2; shift 2
  if "$@" >/dev/null 2>&1; then echo "PASS $n $desc"; pass=$((pass+1)); else echo "FAIL $n $desc"; fail=$((fail+1)); fi
}
q() { jq -e "$2" "out/$1.json"; }

echo; echo "===== checks ====="
check 1 "scan flags Deno.readTextFileSync as missing and shims Deno.serve and Deno.env" \
  q before '[.apis[] | select(.shimmed == false) | .api] == ["Deno.readTextFileSync"] and ([.apis[] | select(.shimmed) | .api] | sort) == ["Deno.env","Deno.serve"]'
check 2 "before: deno serves all 7 requests, workerd refuses to boot on the global-scope UUID, exit 1" \
  bash -c 'jq -e ".deno_boot.ok and ([.results[].deno.status] == [200,200,200,200,200,200,404]) and (.workerd_boot.ok | not) and (.workerd_boot.error | test(\"Disallowed operation called within global scope\"))" out/before.json && [ "$(cat out/before.exit)" = 1 ]'
check 3 "halfway: workerd boots and exactly /whoami, /validate x2, /about differ with the real workerd errors, exit 1" \
  bash -c 'jq -e ".workerd_boot.ok and ([.results[] | select(.same | not) | .name] == [\"GET /whoami\",\"GET /validate good\",\"GET /validate bad\",\"GET /about\"]) and (.results[1].workerd.error | test(\"remoteAddr\")) and (.results[2].workerd.error | test(\"Code generation from strings disallowed\"))" out/halfway.json && [ "$(cat out/halfway.exit)" = 1 ]'
check 4 "halfway: /hello, POST /echo, /missing match, and the env binding reaches Deno.env.get on workerd" \
  q halfway '([.results[] | select(.same) | .name] == ["GET /hello","POST /echo","GET /missing"]) and (.results[0].workerd.body | test("hello from env"))'
check 5 "after: 7/7 requests match on deno and workerd, exit 0" \
  bash -c 'jq -e ".ok and ([.results[] | select(.same)] | length) == 7" out/after.json && [ "$(cat out/after.exit)" = 0 ]'

echo; echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
