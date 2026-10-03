#!/usr/bin/env python3
# N°230 — pilote de migration d'URL d'un routeur (un à la fois).
# Usage : ADMIN_PASSWORD=… python3 ops/migrate-router.py <router-id>
# (identifiants admin plateforme par environnement — rien dans le dépôt.)
import json, os, sys, time, urllib.request, urllib.parse

BASE = os.environ.get("MIKCLOUD_API", "https://api.mikcloud.ftci.fr")

def api(path, data=None, method=None, token=None, timeout=20):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    body = json.dumps(data).encode() if data is not None else None
    req = urllib.request.Request(BASE + path, data=body, method=method or ("POST" if body else "GET"), headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return r.status, json.loads(r.read().decode() or "{}")
    except urllib.error.HTTPError as e:
        try:
            return e.code, json.loads(e.read().decode() or "{}")
        except Exception:
            return e.code, {}

# 1. login admin plateforme
st, resp = api("/api/auth/login", {"username": os.environ.get("ADMIN_USERNAME", "admin"), "password": os.environ.get("ADMIN_PASSWORD", "")})
assert st == 200, f"login {st} {resp}"
tok = resp["token"]
print("login admin OK")

router_id = sys.argv[1]

# 2. état avant
st, r = api(f"/api/admin/fleet/routers", token=tok)
rows = r if isinstance(r, list) else r.get("routers", r.get("data", []))
before = next((x for x in rows if x["id"] == router_id), None)
assert before, "routeur introuvable"
print(f"AVANT   : {before['name']} | statut={before['status']} | lastSeen={before['lastSeen']}")

# 3. migration
st, resp = api(f"/api/routers/{router_id}/migrate-url", {}, token=tok)
assert st == 200, f"migrate-url {st} {resp}"
cmd_id = resp.get("commandId")
print(f"MIGRATION: queued={resp.get('queued')} commandId={cmd_id}")
print(f"          {resp.get('message')}")

# 4. poll du statut (jusqu'à 8 min — veille adaptative 45-240 s + marges)
deadline = time.time() + 480
final = None
while time.time() < deadline:
    st, c = api(f"/api/commands/{cmd_id}", token=tok)
    status = c.get("status")
    if status in ("done", "error"):
        final = c
        break
    time.sleep(15)
assert final, "timeout : commande toujours queued/sent après 8 min"
print(f"RÉSULTAT: status={final['status']}")
if final.get("result"):
    print(f"          result={json.dumps(final['result'], ensure_ascii=False)}")

# 5. le routeur check-in toujours ? (2 lectures espacées)
for i in range(2):
    time.sleep(50)
    st, r = api("/api/admin/fleet/routers", token=tok)
    rows = r if isinstance(r, list) else r.get("routers", r.get("data", []))
    cur = next((x for x in rows if x["id"] == router_id), None)
    print(f"APRÈS{i+1} : statut={cur['status']} | lastSeen={cur['lastSeen']}")
