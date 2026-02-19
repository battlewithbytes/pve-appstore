#!/usr/bin/env bash
# End-to-end API tests for 5 Developer Mode features:
#   1. Catalog Stack Definitions
#   2. Dev Stack Workflow (CRUD + deploy/undeploy)
#   3. Import ZIP (app + stack)
#   4. Cleanup on Disable
#   5. PR Status Tracking
#
# Usage: bash test-e2e.sh
# Requires: pve-appstore service running on localhost:8088

set -euo pipefail

BASE="http://localhost:8088"
PASS=0
FAIL=0
TESTS=()

# ─── Helpers ────────────────────────────────────────────────────────────────

pass() { PASS=$((PASS+1)); TESTS+=("PASS: $1"); echo "  ✓ $1"; }
fail() { FAIL=$((FAIL+1)); TESTS+=("FAIL: $1 — $2"); echo "  ✗ $1: $2"; }

assert_status() {
    local name="$1" expected="$2" actual="$3"
    if [ "$actual" = "$expected" ]; then
        pass "$name (HTTP $expected)"
    else
        fail "$name" "expected HTTP $expected, got $actual"
    fi
}

assert_json() {
    local name="$1" jq_expr="$2" expected="$3" body="$4"
    local actual
    actual=$(echo "$body" | python3 -c "import json,sys; d=json.load(sys.stdin); print($jq_expr)" 2>/dev/null || echo "__ERROR__")
    if [ "$actual" = "$expected" ]; then
        pass "$name"
    else
        fail "$name" "expected '$expected', got '$actual'"
    fi
}

assert_contains() {
    local name="$1" needle="$2" haystack="$3"
    if echo "$haystack" | grep -q "$needle"; then
        pass "$name"
    else
        fail "$name" "response does not contain '$needle'"
    fi
}

curl_get() { curl -sf -w '\n%{http_code}' "$BASE$1" 2>/dev/null || echo -e "\n000"; }
curl_post() { curl -sf -w '\n%{http_code}' -X POST "$BASE$1" "${@:2}" 2>/dev/null || echo -e "\n000"; }
curl_put() { curl -sf -w '\n%{http_code}' -X PUT "$BASE$1" "${@:2}" 2>/dev/null || echo -e "\n000"; }
curl_delete() { curl -sf -w '\n%{http_code}' -X DELETE "$BASE$1" 2>/dev/null || echo -e "\n000"; }

# Split body and status from curl output (status is always last line)
split_response() {
    local resp="$1"
    BODY=$(echo "$resp" | head -n -1)
    STATUS=$(echo "$resp" | tail -n1)
}

# Versions that don't use -f so we can capture error bodies too
curl_get_raw()    { curl -s -w '\n%{http_code}' "$BASE$1" 2>/dev/null || echo -e "\n000"; }
curl_post_raw()   { curl -s -w '\n%{http_code}' -X POST "$BASE$1" "${@:2}" 2>/dev/null || echo -e "\n000"; }
curl_put_raw()    { curl -s -w '\n%{http_code}' -X PUT "$BASE$1" "${@:2}" 2>/dev/null || echo -e "\n000"; }
curl_delete_raw() { curl -s -w '\n%{http_code}' -X DELETE "$BASE$1" 2>/dev/null || echo -e "\n000"; }

# ─── Phase 0: Save Config & Disable Auth ────────────────────────────────────

echo ""
echo "═══════════════════════════════════════════════════════════"
echo "  PVE App Store — E2E Developer Mode Tests"
echo "═══════════════════════════════════════════════════════════"
echo ""

CONFIG="/etc/pve-appstore/config.yml"
ORIG_AUTH=$(grep 'mode:' "$CONFIG" | head -1 | awk '{print $2}')
echo "Phase 0: Setting auth.mode to 'none' (was: $ORIG_AUTH)"

sed -i 's/mode: '"$ORIG_AUTH"'/mode: none/' "$CONFIG"
systemctl restart pve-appstore
echo "  Waiting for service..."
for i in $(seq 1 20); do
    if curl -sf "$BASE/api/health" >/dev/null 2>&1; then break; fi
    sleep 0.5
done
if ! curl -sf "$BASE/api/health" >/dev/null 2>&1; then
    echo "  ERROR: Service did not start. Restoring config and aborting."
    sed -i 's/mode: none/mode: '"$ORIG_AUTH"'/' "$CONFIG"
    systemctl restart pve-appstore
    exit 1
fi
echo "  Service healthy."
echo ""

# Ensure dev mode is ON
curl -s -X PUT "$BASE/api/settings" \
  -H "Content-Type: application/json" \
  -d '{"developer":{"enabled":true}}' >/dev/null

# ─── Phase 1: Catalog Stacks (initially empty) ──────────────────────────────

echo "Phase 1: Catalog Stack Definitions"
echo "───────────────────────────────────"

split_response "$(curl_get_raw /api/catalog-stacks)"
assert_status "GET /api/catalog-stacks" "200" "$STATUS"
assert_json "catalog stacks initially empty or low count" "d.get('total', -1)" "0" "$BODY" || true
# May not be 0 if there are already deployed stacks — just check the endpoint works
if [ "$STATUS" = "200" ]; then
    pass "catalog-stacks endpoint returns 200 OK"
else
    fail "catalog-stacks endpoint" "got status $STATUS"
fi

echo ""

# ─── Phase 2: Import ZIP ────────────────────────────────────────────────────

echo "Phase 2: Import ZIP"
echo "───────────────────"

# 2.1: Export existing dev app "swag" and re-import as "swag-imported"
echo "  [2.1] Export swag → re-import as swag-imported"
EXPORT_STATUS=$(curl -s -o /tmp/swag-export.zip -w '%{http_code}' -X POST "$BASE/api/dev/apps/swag/export")
if [ "$EXPORT_STATUS" = "200" ]; then
    pass "export swag as ZIP (HTTP 200)"

    # Repack with new name
    rm -rf /tmp/zip-repack
    mkdir -p /tmp/zip-repack
    cd /tmp/zip-repack
    unzip -qo /tmp/swag-export.zip 2>/dev/null || true
    if [ -d swag ]; then
        mv swag swag-imported
        zip -qr /tmp/swag-imported.zip swag-imported/
        pass "repacked ZIP as swag-imported"

        split_response "$(curl_post_raw /api/dev/import/zip -F "file=@/tmp/swag-imported.zip")"
        assert_status "import swag-imported ZIP" "200" "$STATUS"
        assert_json "import type is app" "d.get('type')" "app" "$BODY"
        assert_json "import id is swag-imported" "d.get('id')" "swag-imported" "$BODY"
    else
        fail "repack ZIP" "swag directory not found in export"
    fi
    cd /root/appstore
else
    fail "export swag as ZIP" "HTTP $EXPORT_STATUS"
fi

# 2.2: Import a stack ZIP from scratch
echo "  [2.2] Import stack from ZIP"
rm -rf /tmp/test-stack-zip
mkdir -p /tmp/test-stack-zip/my-test-stack
cat > /tmp/test-stack-zip/my-test-stack/stack.yml << 'YAML'
id: my-test-stack
name: Test Import Stack
description: Stack imported from ZIP
version: "0.1.0"
categories:
  - testing
apps:
  - app_id: nginx
lxc:
  ostemplate: "local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst"
  defaults:
    cores: 1
    memory_mb: 512
    disk_gb: 4
YAML
cd /tmp/test-stack-zip && zip -qr /tmp/test-stack.zip my-test-stack/
cd /root/appstore

split_response "$(curl_post_raw /api/dev/import/zip -F "file=@/tmp/test-stack.zip")"
assert_status "import stack ZIP" "200" "$STATUS"
assert_json "import type is stack" "d.get('type')" "stack" "$BODY"
assert_json "import id is my-test-stack" "d.get('id')" "my-test-stack" "$BODY"

# 2.3: Error cases
echo "  [2.3] Error cases"

# No file
split_response "$(curl_post_raw /api/dev/import/zip)"
assert_status "import no file → 400" "400" "$STATUS"

# Invalid ZIP
echo "not a zip" > /tmp/bad.zip
split_response "$(curl_post_raw /api/dev/import/zip -F "file=@/tmp/bad.zip")"
assert_status "import bad ZIP → 400" "400" "$STATUS"

# Duplicate
split_response "$(curl_post_raw /api/dev/import/zip -F "file=@/tmp/test-stack.zip")"
assert_status "import duplicate → 409" "409" "$STATUS"

echo ""

# ─── Phase 3: Dev Stack Workflow ─────────────────────────────────────────────

echo "Phase 3: Dev Stack Workflow"
echo "───────────────────────────"

# 3.1: Create stack from template
echo "  [3.1] Create stack"
split_response "$(curl_post_raw /api/dev/stacks -H 'Content-Type: application/json' -d '{"id":"e2e-stack","template":"web-database"}')"
assert_status "create e2e-stack" "201" "$STATUS"
assert_json "created stack has manifest" "bool(d.get('manifest',''))" "True" "$BODY"

# 3.2: List stacks
echo "  [3.2] List stacks"
split_response "$(curl_get_raw /api/dev/stacks)"
assert_status "list dev stacks" "200" "$STATUS"
# Should contain both e2e-stack and my-test-stack
assert_contains "list contains e2e-stack" "e2e-stack" "$BODY"
assert_contains "list contains my-test-stack" "my-test-stack" "$BODY"

# 3.3: Get specific stack
echo "  [3.3] Get stack detail"
split_response "$(curl_get_raw /api/dev/stacks/e2e-stack)"
assert_status "get e2e-stack" "200" "$STATUS"
assert_contains "stack has manifest content" "web-database" "$BODY"

# 3.4: Save updated manifest
echo "  [3.4] Save manifest"
split_response "$(curl_put_raw /api/dev/stacks/e2e-stack/manifest \
  -H 'Content-Type: application/octet-stream' \
  -d 'id: e2e-stack
name: E2E Test Stack
description: End-to-end test stack
version: "1.0.0"
categories:
  - testing
tags:
  - e2e
apps:
  - app_id: nginx
  - app_id: hello-world
lxc:
  ostemplate: "local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst"
  defaults:
    cores: 2
    memory_mb: 1024
    disk_gb: 8')"
assert_status "save stack manifest" "200" "$STATUS"
assert_json "save returns saved" "d.get('status')" "saved" "$BODY"

# 3.5: Validate stack
echo "  [3.5] Validate stack"
split_response "$(curl_post_raw /api/dev/stacks/e2e-stack/validate)"
assert_status "validate stack" "200" "$STATUS"
assert_json "validation result has valid field" "str(type(d.get('valid','missing')))" "<class 'bool'>" "$BODY"

# 3.6: Deploy to catalog
echo "  [3.6] Deploy stack"
split_response "$(curl_post_raw /api/dev/stacks/e2e-stack/deploy)"
assert_status "deploy stack" "200" "$STATUS"
assert_json "deploy returns deployed" "d.get('status')" "deployed" "$BODY"

# Verify in catalog
split_response "$(curl_get_raw /api/catalog-stacks)"
assert_contains "catalog contains deployed stack" "e2e-stack" "$BODY"

# Get specific catalog stack
split_response "$(curl_get_raw /api/catalog-stacks/e2e-stack)"
assert_status "get catalog stack detail" "200" "$STATUS"
assert_contains "catalog stack has apps" "nginx" "$BODY"

# 3.7: Export stack
echo "  [3.7] Export stack"
EXPORT_STATUS=$(curl -s -o /tmp/e2e-stack-export.zip -w '%{http_code}' -X POST "$BASE/api/dev/stacks/e2e-stack/export")
assert_status "export e2e-stack as ZIP" "200" "$EXPORT_STATUS"
if [ -f /tmp/e2e-stack-export.zip ]; then
    if unzip -l /tmp/e2e-stack-export.zip 2>/dev/null | grep -q "stack.yml"; then
        pass "export ZIP contains stack.yml"
    else
        fail "export ZIP contents" "stack.yml not found in ZIP"
    fi
else
    fail "export ZIP file" "file not created"
fi

# 3.8: Undeploy stack
echo "  [3.8] Undeploy stack"
split_response "$(curl_post_raw /api/dev/stacks/e2e-stack/undeploy)"
assert_status "undeploy stack" "200" "$STATUS"
assert_json "undeploy returns undeployed" "d.get('status')" "undeployed" "$BODY"

# Verify removed from catalog
split_response "$(curl_get_raw /api/catalog-stacks/e2e-stack)"
assert_status "undeployed stack not in catalog → 404" "404" "$STATUS"

# 3.9: Delete stack
echo "  [3.9] Delete stack"
split_response "$(curl_delete_raw /api/dev/stacks/e2e-stack)"
assert_status "delete e2e-stack" "200" "$STATUS"
assert_json "delete returns deleted" "d.get('status')" "deleted" "$BODY"

# Verify gone
split_response "$(curl_get_raw /api/dev/stacks/e2e-stack)"
assert_status "deleted stack → 404" "404" "$STATUS"

echo ""

# ─── Phase 4: Cleanup on Disable ────────────────────────────────────────────

echo "Phase 4: Cleanup on Disable"
echo "───────────────────────────"

# Create and deploy a stack for cleanup testing
echo "  [4.1] Setup: create + deploy cleanup-stack"
curl_post_raw /api/dev/stacks -H 'Content-Type: application/json' \
  -d '{"id":"cleanup-stack","template":"blank"}' >/dev/null

split_response "$(curl_put_raw /api/dev/stacks/cleanup-stack/manifest \
  -H 'Content-Type: application/octet-stream' \
  -d 'id: cleanup-stack
name: Cleanup Test
description: Test cleanup on disable
version: "0.1.0"
categories:
  - testing
apps:
  - app_id: nginx
lxc:
  ostemplate: "local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst"
  defaults:
    cores: 1
    memory_mb: 512
    disk_gb: 4')"

split_response "$(curl_post_raw /api/dev/stacks/cleanup-stack/deploy)"
assert_status "deploy cleanup-stack" "200" "$STATUS"

# Verify it's in catalog
split_response "$(curl_get_raw /api/catalog-stacks)"
assert_contains "cleanup-stack in catalog before disable" "cleanup-stack" "$BODY"

# Disable dev mode
echo "  [4.2] Disable developer mode"
split_response "$(curl_put_raw /api/settings -H 'Content-Type: application/json' -d '{"developer":{"enabled":false}}')"
assert_status "disable dev mode" "200" "$STATUS"

# Check catalog stacks cleared
split_response "$(curl_get_raw /api/catalog-stacks)"
STACK_COUNT=$(echo "$BODY" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('total',0))" 2>/dev/null || echo "-1")
if [ "$STACK_COUNT" = "0" ]; then
    pass "all dev stacks removed from catalog after disable"
else
    fail "cleanup stacks" "expected 0 catalog stacks, got $STACK_COUNT"
fi

# Check dev apps not in catalog with source=developer
split_response "$(curl_get_raw /api/apps)"
DEV_APP_COUNT=$(echo "$BODY" | python3 -c "
import json,sys
d=json.load(sys.stdin)
devs=[a['id'] for a in d.get('apps',[]) if a.get('source')=='developer']
print(len(devs))
" 2>/dev/null || echo "-1")
if [ "$DEV_APP_COUNT" = "0" ]; then
    pass "all dev apps removed from catalog after disable"
else
    fail "cleanup apps" "expected 0 dev apps in catalog, got $DEV_APP_COUNT"
fi

# Re-enable dev mode
echo "  [4.3] Re-enable developer mode"
split_response "$(curl_put_raw /api/settings -H 'Content-Type: application/json' -d '{"developer":{"enabled":true}}')"
assert_status "re-enable dev mode" "200" "$STATUS"

# Verify stack status is now "draft"
split_response "$(curl_get_raw /api/dev/stacks)"
CLEANUP_STATUS=$(echo "$BODY" | python3 -c "
import json,sys
d=json.load(sys.stdin)
for s in d.get('stacks',[]):
    if s['id']=='cleanup-stack':
        print(s.get('status','unknown'))
        break
else:
    print('not_found')
" 2>/dev/null || echo "error")
if [ "$CLEANUP_STATUS" = "draft" ]; then
    pass "cleanup-stack status reset to draft after re-enable"
else
    fail "stack status after re-enable" "expected 'draft', got '$CLEANUP_STATUS'"
fi

echo ""

# ─── Phase 5: PR Status Tracking ────────────────────────────────────────────

echo "Phase 5: PR Status Tracking"
echo "───────────────────────────"

# 5.1: App publish status
echo "  [5.1] App publish status"
split_response "$(curl_get_raw /api/dev/apps/swag/publish-status)"
assert_status "GET app publish-status" "200" "$STATUS"
assert_json "publish-status has checks" "str(type(d.get('checks')))" "<class 'dict'>" "$BODY"
assert_json "publish-status has ready field" "str(type(d.get('ready')))" "<class 'bool'>" "$BODY"
assert_json "publish-status has published field" "str(type(d.get('published')))" "<class 'bool'>" "$BODY"

# 5.2: GitHub connection status
echo "  [5.2] GitHub connection status"
split_response "$(curl_get_raw /api/dev/github/status)"
assert_status "GET github status" "200" "$STATUS"
assert_json "github status has connected field" "str(type(d.get('connected')))" "<class 'bool'>" "$BODY"

# 5.3: Stack publish status
echo "  [5.3] Stack publish status"
split_response "$(curl_get_raw /api/dev/stacks/cleanup-stack/publish-status)"
assert_status "GET stack publish-status" "200" "$STATUS"
assert_json "stack publish-status has checks" "str(type(d.get('checks')))" "<class 'dict'>" "$BODY"
assert_json "stack publish-status has ready" "str(type(d.get('ready')))" "<class 'bool'>" "$BODY"

# Verify checks keys exist
STACK_CHECKS=$(echo "$BODY" | python3 -c "
import json,sys
d=json.load(sys.stdin)
checks=d.get('checks',{})
keys=sorted(checks.keys())
print(','.join(keys))
" 2>/dev/null || echo "error")
assert_contains "stack checks include apps_published" "apps_published" "$STACK_CHECKS"
assert_contains "stack checks include github_connected" "github_connected" "$STACK_CHECKS"
assert_contains "stack checks include validation_passed" "validation_passed" "$STACK_CHECKS"

echo ""

# ─── Phase 6: Cleanup Test Data ─────────────────────────────────────────────

echo "Phase 6: Cleanup"
echo "────────────────"

echo "  Deleting test stacks and apps..."
curl -s -X DELETE "$BASE/api/dev/stacks/my-test-stack" >/dev/null 2>&1 || true
curl -s -X DELETE "$BASE/api/dev/stacks/cleanup-stack" >/dev/null 2>&1 || true
curl -s -X DELETE "$BASE/api/dev/stacks/e2e-stack" >/dev/null 2>&1 || true

# Delete imported app
# Dev apps don't have a DELETE endpoint — remove manually
if [ -d "/var/lib/pve-appstore/dev-apps/swag-imported" ]; then
    rm -rf "/var/lib/pve-appstore/dev-apps/swag-imported"
    echo "  Removed swag-imported dev app directory"
fi

rm -f /tmp/swag-export.zip /tmp/swag-imported.zip /tmp/test-stack.zip \
      /tmp/e2e-stack-export.zip /tmp/bad.zip
rm -rf /tmp/zip-repack /tmp/test-stack-zip
echo "  Temp files cleaned."

echo ""

# ─── Phase 7: Restore Auth Config ───────────────────────────────────────────

echo "Phase 7: Restoring auth config"
echo "──────────────────────────────"

sed -i 's/mode: none/mode: '"$ORIG_AUTH"'/' "$CONFIG"
systemctl restart pve-appstore
echo "  Waiting for service..."
for i in $(seq 1 20); do
    if curl -sf "$BASE/api/health" >/dev/null 2>&1; then break; fi
    sleep 0.5
done
if curl -sf "$BASE/api/health" >/dev/null 2>&1; then
    echo "  Service restored with auth.mode: $ORIG_AUTH"
else
    echo "  WARNING: Service did not restart cleanly!"
fi

echo ""

# ─── Summary ────────────────────────────────────────────────────────────────

echo "═══════════════════════════════════════════════════════════"
echo "  RESULTS: $PASS passed, $FAIL failed"
echo "═══════════════════════════════════════════════════════════"
echo ""

if [ $FAIL -gt 0 ]; then
    echo "Failures:"
    for t in "${TESTS[@]}"; do
        if [[ "$t" == FAIL* ]]; then
            echo "  $t"
        fi
    done
    echo ""
    exit 1
fi

echo "All tests passed!"
exit 0
