#!/usr/bin/env python3
# MIKCLOUD — Étape 8 du 1er octobre (RUNBOOK-POSTGRES.md §11) : retourner le
# secret GitHub DATABASE_URL vers la valeur SUPABASE (il servait de SOURCE à
# la migration Neon → Supabase ; après la bascule il doit désigner la
# production Supabase, sinon backup.yml exporterait l'ancienne base).
#
# Mécanique : clé publique du dépôt (X25519) + sealed box libsodium
# (PyNaCl) + PUT /repos/{repo}/actions/secrets/DATABASE_URL.
#
# Usage :
#   python3 ops/oct1/step8-flip-secret.py            # DRY-RUN
#   python3 ops/oct1/step8-flip-secret.py --exec     # APPLIQUE
#
# Secrets (env > coffre /home/z/.secrets) :
#   GITHUB_TOKEN           PAT avec droits repo (ghp_…)
#   SUPABASE_DATABASE_URL  DSN session pooler :5432 SANS sslmode=
#
# Dépendance : PyNaCl — uv pip install pynacl
import json
import os
import sys
import urllib.request

REPO = "ftechnologies18/mikcloud"
VAULT = os.environ.get("MIKCLOUD_VAULT", "/home/z/.secrets")


def die(msg: str) -> None:
    print(f"✗ {msg}", file=sys.stderr)
    sys.exit(1)


def pick(env: str, vault_file: str, pattern: str) -> str:
    v = os.environ.get(env, "")
    if not v:
        p = os.path.join(VAULT, vault_file)
        if os.path.exists(p):
            for line in open(p):
                line = line.strip()
                if pattern in line and not line.startswith("#"):
                    return line
    return v


def gh(method: str, path: str, token: str, body: bytes | None = None):
    req = urllib.request.Request(
        f"https://api.github.com{path}",
        method=method,
        data=body,
        headers={
            "Authorization": f"Bearer {token}",
            "Accept": "application/vnd.github+json",
            "Content-Type": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=20) as r:
            return r.status, r.read()
    except urllib.error.HTTPError as e:
        return e.code, e.read()


def main() -> None:
    exec_mode = "--exec" in sys.argv[1:]
    token = pick("GITHUB_TOKEN", "github-token.txt", "ghp_")
    dsn = pick("SUPABASE_DATABASE_URL", "supabase-dsn.txt", ":5432/")
    if not token:
        die("GITHUB_TOKEN absent (env ou coffre github-token.txt)")
    if not dsn:
        die("SUPABASE_DATABASE_URL absent (env ou coffre supabase-dsn.txt, ligne :5432)")

    # Validations dures (miroir de step5-render-flip.sh)
    if ".pooler.supabase.com:5432/" not in dsn:
        die("Le DSN doit pointer le session pooler .pooler.supabase.com:5432")
    if "sslmode=" in dsn or "pgbouncer=" in dsn or ":6543" in dsn:
        die("DSN invalide (sslmode/pgbouncer/6543 interdits ici)")

    try:
        from nacl import encoding, public
    except ImportError:
        die("PyNaCl absent — installer : uv pip install pynacl")

    print("═══ Étape 8a — flip du secret GitHub DATABASE_URL → Supabase ═══")
    if not exec_mode:
        print("MODE DRY-RUN (aucune écriture — ajouter --exec pour appliquer)")

    # Clé publique du dépôt
    status, body = gh("GET", f"/repos/{REPO}/actions/secrets/public-key", token)
    if status != 200:
        die(f"clé publique du dépôt : HTTP {status} — {body[:200]!r}")
    pk = json.loads(body)
    print(f"  · clé publique du dépôt : id={pk['key_id']}")

    # Chiffrement (sealed box) puis PUT
    sealed = public.SealedBox(public.PublicKey(pk["key"].encode(), encoding.Base64Encoder()))
    enc = sealed.encrypt(dsn.encode())
    payload = json.dumps({
        "encrypted_value": encoding.Base64Encoder.encode(enc).decode(),
        "key_id": pk["key_id"],
    }).encode()

    if not exec_mode:
        print("  · PUT /repos/%s/actions/secrets/DATABASE_URL (DRY-RUN — non exécuté)" % REPO)
        print("DRY-RUN terminé — rien n'a été modifié.")
        return

    status, body = gh("PUT", f"/repos/{REPO}/actions/secrets/DATABASE_URL", token, payload)
    if status not in (201, 204):
        die(f"PUT secret : HTTP {status} — {body[:200]!r}")
    print("  ✓ secret DATABASE_URL posé (valeur Supabase session pooler :5432)")

    # Vérification : l'horodatage du secret a bougé
    status, body = gh("GET", f"/repos/{REPO}/actions/secrets", token)
    if status == 200:
        for s in json.loads(body).get("secrets", []):
            if s["name"] == "DATABASE_URL":
                print(f"  ✓ vérifié : DATABASE_URL mis à jour à {s['updated_at']}")
    print("═══ Étape 8a TERMINÉE ═══")


if __name__ == "__main__":
    main()
