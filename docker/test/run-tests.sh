#!/usr/bin/env bash
# Integration test suite for davit v0.1
# Runs inside a fresh Ubuntu 24.04 container.
set -euo pipefail

PASS=0
FAIL=0
SKIP=0

# ── helpers ────────────────────────────────────────────────────────────────────

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

section() { echo; echo -e "${BOLD}${CYAN}══ $* ══${RESET}"; }
pass()    { echo -e "  ${GREEN}✓${RESET} $*"; PASS=$((PASS+1)); }
fail()    { echo -e "  ${RED}✗${RESET} $*"; FAIL=$((FAIL+1)); }
skip()    { echo -e "  ${YELLOW}⊘${RESET} $* (skipped — container limitation)"; SKIP=$((SKIP+1)); }
info()    { echo -e "    ${CYAN}↳${RESET} $*"; }

assert_exit0()   { local desc="$1"; shift; if "$@" >/dev/null 2>&1; then pass "$desc"; else fail "$desc"; fi; }
assert_contains() {
    local desc="$1" pattern="$2"; shift 2
    local out; out=$("$@" 2>&1)
    if echo "$out" | grep -q "$pattern"; then
        pass "$desc"
    else
        fail "$desc (expected pattern: '$pattern')"
        info "actual output: $out"
    fi
}
assert_json_field() {
    local desc="$1" field="$2" expected="$3"; shift 3
    local out; out=$("$@" 2>&1)
    local actual; actual=$(echo "$out" | grep -o "\"${field}\":[^,}]*" | head -1 | sed 's/.*: *//' | tr -d '"')
    if [ "$actual" = "$expected" ]; then
        pass "$desc"
    else
        fail "$desc (field '$field': expected '$expected', got '$actual')"
        info "actual output: $out"
    fi
}

davit() { /usr/local/bin/davit "$@"; }

# ── Setup ──────────────────────────────────────────────────────────────────────

section "Environment"

# Confirm we're on Ubuntu
. /etc/os-release
info "OS: $PRETTY_NAME"
info "Arch: $(uname -m)"
info "Kernel: $(uname -r)"

assert_contains "OS is Ubuntu" "Ubuntu" cat /etc/os-release
assert_exit0    "davit binary is executable" test -x /usr/local/bin/davit

# ── Version ────────────────────────────────────────────────────────────────────

section "davit --version"

assert_contains "prints version string"    "v0.1.0"  davit --version
assert_contains "prints 'davit' in output" "davit"   davit --version
info "$(davit --version)"

# ── Help / command tree ────────────────────────────────────────────────────────

section "Command tree"

assert_contains "root help shows 'server'"  "server"  davit --help
assert_contains "root help shows 'app'"     "app"     davit --help
assert_contains "root help shows 'agent'"   "agent"   davit --help
assert_contains "server subcommands"        "init"    davit server --help
assert_contains "server subcommands"        "status"  davit server --help
assert_contains "app subcommands"           "create"  davit app --help
assert_contains "app subcommands"           "deploy"  davit app --help
assert_contains "app subcommands"           "list"    davit app --help
assert_contains "agent subcommands"         "create"  davit agent key --help

# ── OS detection (dry-run provisioning) ───────────────────────────────────────

section "davit server init --dry-run"

info "Running full dry-run provisioner..."
DRY_OUT=$(davit --json server init \
    --email admin@example.com \
    --timezone Europe/London \
    --dry-run 2>&1)

echo "$DRY_OUT" | grep '"step"' | while IFS= read -r line; do
    step=$(echo "$line" | grep -o '"step":"[^"]*"' | cut -d'"' -f4)
    status=$(echo "$line" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
    msg=$(echo "$line" | grep -o '"message":"[^"]*"' | cut -d'"' -f4)
    info "  [$status] $step — $msg"
done

if echo "$DRY_OUT" | grep -q '"status":"ok"'; then
    pass "dry-run completed with status ok"
else
    fail "dry-run did not complete successfully"
    info "$DRY_OUT"
fi

STEPS_TOTAL=$(echo "$DRY_OUT" | grep -o '"steps_total":[0-9]*' | grep -o '[0-9]*$')
info "steps_total: $STEPS_TOTAL"
if [ "$STEPS_TOTAL" = "11" ]; then
    pass "all 11 provisioning steps present"
else
    fail "expected 11 steps, got: $STEPS_TOTAL"
fi

# Verify each expected step name appears
for step in system_update install_core_utils configure_timezone ssh_hardening \
            configure_firewall install_fail2ban install_docker install_caddy \
            create_dir_structure init_state_db install_daemon_unit; do
    if echo "$DRY_OUT" | grep -q "\"$step\""; then
        pass "step '$step' present"
    else
        fail "step '$step' missing from dry-run output"
    fi
done

# ── Real partial init (container-safe steps only) ─────────────────────────────

section "davit server init (real — container-safe steps)"

info "Running real init skipping steps that need kernel capabilities..."
info "(skipping: configure_firewall, install_docker, install_caddy, install_daemon_unit)"

INIT_OUT=$(davit --json server init \
    --email admin@example.com \
    --timezone UTC \
    --skip-steps "configure_firewall,install_docker,install_caddy,install_daemon_unit" \
    2>&1)

if echo "$INIT_OUT" | grep -q '"status":"ok"'; then
    pass "partial init completed with status ok"
else
    fail "partial init failed"
    info "$INIT_OUT"
fi

# Check the steps that should have run
for step in system_update install_core_utils configure_timezone; do
    if echo "$INIT_OUT" | grep -q "\"$step\"" && echo "$INIT_OUT" | grep -A2 "\"$step\"" | grep -q '"ok"'; then
        pass "step '$step' ran and passed"
    else
        fail "step '$step' did not pass"
    fi
done

# SSH hardening should warn (no SSH key in container), not error
if echo "$INIT_OUT" | grep -q "WARN_NO_SSH_KEY"; then
    pass "ssh_hardening correctly warns about missing SSH key (no lockout)"
else
    info "ssh_hardening output: $(echo "$INIT_OUT" | grep ssh_hardening || echo 'not found')"
fi

# Skipped steps should appear as skipped
for step in configure_firewall install_docker install_caddy install_daemon_unit; do
    if echo "$INIT_OUT" | grep "\"$step\"" | grep -q '"skipped"'; then
        pass "step '$step' correctly skipped"
    else
        fail "step '$step' was not skipped as expected"
    fi
done

# ── server status ──────────────────────────────────────────────────────────────

section "davit server status"

STATUS_OUT=$(davit --json server status 2>&1)
info "$STATUS_OUT"

if echo "$STATUS_OUT" | grep -q '"status":"ok"'; then
    pass "server status returns ok"
else
    fail "server status did not return ok"
fi

# Check expected fields
for field in hostname os arch davit_version provisioned apps_total; do
    if echo "$STATUS_OUT" | grep -q "\"$field\""; then
        pass "status field '$field' present"
    else
        fail "status field '$field' missing"
    fi
done

HOSTNAME=$(echo "$STATUS_OUT" | grep -o '"hostname":"[^"]*"' | cut -d'"' -f4)
info "hostname: $HOSTNAME"

ARCH=$(echo "$STATUS_OUT" | grep -o '"arch":"[^"]*"' | cut -d'"' -f4)
info "arch: $ARCH"

OS=$(echo "$STATUS_OUT" | grep -o '"os":"[^"]*"' | cut -d'"' -f4)
info "os: $OS"

PROVISIONED=$(echo "$STATUS_OUT" | grep -o '"provisioned":[a-z]*' | cut -d: -f2)
info "provisioned: $PROVISIONED"
if [ "$PROVISIONED" = "true" ]; then
    pass "server marked as provisioned after init"
else
    fail "server not marked as provisioned"
fi

# ── agent key create ───────────────────────────────────────────────────────────

section "davit agent key create"

mkdir -p /tmp/davit-keys
KEY_OUT=$(davit --json agent key create \
    --label "test-agent" \
    --output /tmp/davit-keys \
    2>&1)
info "$KEY_OUT"

if echo "$KEY_OUT" | grep -q '"status":"ok"'; then
    pass "agent key create succeeded"
else
    fail "agent key create failed"
fi

# Check files were written
if [ -f /tmp/davit-keys/davit-agent.pem ]; then
    PERMS=$(stat -c '%a' /tmp/davit-keys/davit-agent.pem)
    pass "private key file written"
    if [ "$PERMS" = "600" ]; then
        pass "private key has correct mode 0600"
    else
        fail "private key mode is $PERMS (expected 600)"
    fi
else
    fail "private key file not found"
fi

if [ -f /tmp/davit-keys/davit-agent.pub ]; then
    pass "public key file written"
    info "public key: $(cat /tmp/davit-keys/davit-agent.pub)"
else
    fail "public key file not found"
fi

# Verify authorized_keys was updated with forced-command
AUTH_KEYS=/root/.ssh/authorized_keys
if [ -f "$AUTH_KEYS" ] && grep -q "command=.*davit.*--json" "$AUTH_KEYS"; then
    pass "authorized_keys contains forced-command entry"
    info "entry: $(grep 'davit' $AUTH_KEYS | head -1)"
else
    fail "forced-command entry not found in authorized_keys"
fi

# Check JSON fields
for field in label public_key fingerprint authorized_keys_entry; do
    if echo "$KEY_OUT" | grep -q "\"$field\""; then
        pass "key output field '$field' present"
    else
        fail "key output field '$field' missing"
    fi
done

FINGERPRINT=$(echo "$KEY_OUT" | grep -o '"fingerprint":"[^"]*"' | cut -d'"' -f4)
info "fingerprint: $FINGERPRINT"

# ── app create ────────────────────────────────────────────────────────────────

section "davit app create"

# Use a real public repo for the git ls-remote reachability check
APP_OUT=$(davit --json app create myapi \
    --repo https://github.com/getdavit/davit \
    --domain api.example.com \
    --branch main \
    --port 3000 \
    2>&1)
info "$APP_OUT"

if echo "$APP_OUT" | grep -q '"status":"ok"'; then
    pass "app create succeeded"
else
    fail "app create failed"
    info "output: $APP_OUT"
fi

for field in name repo branch domain port internal_port created_at; do
    if echo "$APP_OUT" | grep -q "\"$field\""; then
        pass "app create field '$field' present"
    else
        fail "app create field '$field' missing"
    fi
done

INTERNAL_PORT=$(echo "$APP_OUT" | grep -o '"internal_port":[0-9]*' | grep -o '[0-9]*$')
info "auto-assigned internal port: $INTERNAL_PORT"
if [ "$INTERNAL_PORT" -ge 40000 ] && [ "$INTERNAL_PORT" -le 49999 ]; then
    pass "internal port in configured range (40000–49999)"
else
    fail "internal port $INTERNAL_PORT out of range"
fi

# Duplicate create should fail with APP_ALREADY_EXISTS
DUP_OUT=$(davit --json app create myapi \
    --repo https://github.com/getdavit/davit \
    --domain api2.example.com \
    2>&1 || true)
if echo "$DUP_OUT" | grep -q "APP_ALREADY_EXISTS"; then
    pass "duplicate app name rejected with APP_ALREADY_EXISTS"
else
    fail "duplicate app name not rejected"
    info "$DUP_OUT"
fi

# Invalid name should fail
INVALID_OUT=$(davit --json app create "MY INVALID NAME" \
    --repo https://github.com/getdavit/davit \
    --domain test.example.com \
    2>&1 || true)
if echo "$INVALID_OUT" | grep -qiE "error|invalid"; then
    pass "invalid app name rejected"
else
    fail "invalid app name not rejected"
    info "$INVALID_OUT"
fi

# ── app list ──────────────────────────────────────────────────────────────────

section "davit app list"

LIST_OUT=$(davit --json app list 2>&1)
info "$LIST_OUT"

if echo "$LIST_OUT" | grep -q '"apps"'; then
    pass "app list returns 'apps' field"
else
    fail "app list missing 'apps' field"
fi

if echo "$LIST_OUT" | grep -q '"myapi"'; then
    pass "created app 'myapi' appears in list"
else
    fail "created app 'myapi' not in list"
fi

APP_COUNT=$(echo "$LIST_OUT" | grep -o '"name"' | wc -l | tr -d ' ')
info "apps in list: $APP_COUNT"
if [ "$APP_COUNT" -ge 1 ]; then
    pass "at least one app listed"
else
    fail "no apps in list"
fi

# ── error format ──────────────────────────────────────────────────────────────

section "Error format"

ERR_OUT=$(davit --json app create notexist \
    --repo https://github.com/getdavit/davit \
    --domain test.example.com \
    2>&1 || true)

# Try a known-error case: app not found
NF_OUT=$(davit --json app deploy doesnotexist 2>&1 || true)
info "$NF_OUT"

for field in status error_code message docs_url; do
    if echo "$NF_OUT" | grep -q "\"$field\""; then
        pass "error envelope field '$field' present"
    else
        fail "error envelope field '$field' missing"
    fi
done

if echo "$NF_OUT" | grep -q '"error_code":"APP_NOT_FOUND"'; then
    pass "APP_NOT_FOUND error code returned"
else
    fail "expected APP_NOT_FOUND error code"
    info "$NF_OUT"
fi

DOCS_URL=$(echo "$NF_OUT" | grep -o '"docs_url":"[^"]*"' | cut -d'"' -f4)
info "docs_url: $DOCS_URL"
if echo "$DOCS_URL" | grep -q "github.com/getdavit/davit"; then
    pass "docs_url points to correct GitHub org"
else
    fail "docs_url incorrect: $DOCS_URL"
fi

# ── idempotency check ─────────────────────────────────────────────────────────

section "Idempotency"

info "Re-running server init (should skip already-complete steps)..."
IDEM_OUT=$(davit --json server init \
    --email admin@example.com \
    --skip-steps "configure_firewall,install_docker,install_caddy,install_daemon_unit" \
    2>&1 || true)

if echo "$IDEM_OUT" | grep -q '"status":"ok"'; then
    pass "re-run of server init succeeded"
else
    fail "re-run of server init failed"
fi

# ── Summary ───────────────────────────────────────────────────────────────────

echo
echo -e "${BOLD}══ Results ══${RESET}"
echo -e "  ${GREEN}Passed:${RESET}  $PASS"
echo -e "  ${RED}Failed:${RESET}  $FAIL"
echo -e "  ${YELLOW}Skipped:${RESET} $SKIP"
echo

if [ "$FAIL" -eq 0 ]; then
    echo -e "${GREEN}${BOLD}All tests passed.${RESET}"
    exit 0
else
    echo -e "${RED}${BOLD}$FAIL test(s) failed.${RESET}"
    exit 1
fi
