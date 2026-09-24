#!/usr/bin/env python3
"""fa-subset.py — N°187 — régénération des polices Font Awesome du portail
captif (webfonts/fa-solid-900.woff2 + fa-brands-400.woff2).

CONTEXTE (incident N°187) : N°75 a amaigri le portail en sous-ensemblant les
polices FA aux seules icônes codées en dur dans les 8 pages (37 glyphes).
N°137 a ajouté la whitelist des 20 icônes « Nos Services » SANS régénérer la
police → 14 glyphes manquants → cases à icônes vides sur les portails des
gérants. Ce script referme la boucle : il reconstruit le sous-ensemble à
partir des DEUX sources de vérité —

  1. les classes fa-* référencées par les 8 pages du template (HTML + JS),
     famille comprise (fab = brands, sinon solid) ;
  2. la whitelist hotpage.PortalServiceIcons (extraite du source Go, aucune
     copie à maintenir).

— puis il écrit le manifeste webfonts_glyphs.json (sha256 + liste des
glyphes) que le test gardien hotpage.TestPortalWebfontsCoverIcons vérifie à
chaque CI : pages + whitelist couvertes, polices embarquées = manifeste.

USAGE
  python3 ops/portal/fa-subset.py                 # polices complètes en cache
  python3 ops/portal/fa-subset.py --download      # (re)télécharger FA 6.4.0
  python3 ops/portal/fa-subset.py --solid P --brands P   # chemins locaux

Prérequis : pip install fonttools brotli. Les polices FA 6.4.0 complètes
ne vivent PAS dans le dépôt (150 Ko + 108 Ko) — cache ~/.cache/mikcloud-fa/
ou --download. La version doit rester 6.4.0 : elle appaire avec le
template/css/all.min.css embarqué (les points de code en dépendent).

RÉSULTAT DÉPLOYÉ : ~5-6 Ko ajoutés au portail par routeur (flash MikroTik) ;
la signature de contenu (hotpage.Sig) re-pousse automatiquement les fichiers
vers les routeurs au check-in suivant — aucune action manuelle.
"""
import argparse
import hashlib
import json
import re
import sys
import urllib.request
from pathlib import Path

from fontTools import subset
from fontTools.ttLib import TTFont

ROOT = Path(__file__).resolve().parents[2]
TEMPLATE = ROOT / "backend/internal/hotpage/template"
CSS = TEMPLATE / "css/all.min.css"
PAGES_DIR = TEMPLATE
WHITELIST_GO = ROOT / "backend/internal/hotpage/serviceicons.go"
MANIFEST = ROOT / "backend/internal/hotpage/webfonts_glyphs.json"
OUT_SOLID = TEMPLATE / "webfonts/fa-solid-900.woff2"
OUT_BRANDS = TEMPLATE / "webfonts/fa-brands-400.woff2"
CACHE = Path.home() / ".cache/mikcloud-fa"
FA_VERSION = "6.4.0"
CDN = "https://cdnjs.cloudflare.com/ajax/libs/font-awesome"
# Classes utilitaires FA (animation/taille/stack) — pas des icônes :
# jamais de glyphe, exclues de la couverture exigée.
UTILITIES = {
    "fa-spin", "fa-spin-pulse", "fa-spin-reverse", "fa-pulse", "fa-fw",
    "fa-border", "fa-inverse", "fa-l", "fa-xs", "fa-sm", "fa-lg", "fa-xl",
    "fa-2x", "fa-3x", "fa-4x", "fa-5x", "fa-6x", "fa-7x", "fa-8x", "fa-9x",
    "fa-stack", "fa-stack-1x", "fa-stack-2x", "fa-sr-only",
    "fa-sr-only-focusable", "fa-flip-horizontal", "fa-flip-vertical",
    "fa-flip-both", "fa-rotate-90", "fa-rotate-180", "fa-rotate-270",
    "fa-rotate-by", "fa-beat", "fa-fade", "fa-beat-fade", "fa-bounce",
    "fa-shake", "fa-normal",
}

TOKEN_RE = re.compile(r"fa-[a-z0-9-]+")
SOLID_PREFIX_RE = re.compile(r"(?:fas|fa-solid)\s+(fa-[a-z0-9-]+)")
BRAND_RE = re.compile(r"(?:fab|fa-brands)\s+(fa-[a-z0-9-]+)")
# Règles all.min.css : `.fa-x:before,.fa-y:before{content:"\fXXX"}` — les
# alias FA6 partagent une règle multi-sélecteurs (piège N°75 : une extraction
# mono-sélecteur ratait 9 icônes).
CSS_RULE_RE = re.compile(r'([^{}]+)\{content:"\\([0-9a-f]+)"\}')
WHITELIST_RE = re.compile(r'"(fa-[a-z0-9-]+)":\s*true')


def fail(msg: str) -> None:
    print(f"ERREUR : {msg}", file=sys.stderr)
    sys.exit(1)


def css_class_to_codepoint() -> dict[str, int]:
    css = CSS.read_text(encoding="utf-8")
    out: dict[str, int] = {}
    for selectors, hexpt in CSS_RULE_RE.findall(css):
        cp = int(hexpt, 16)
        for sel in selectors.split(","):
            m = re.fullmatch(r"\.(fa-[a-z0-9-]+):before", sel.strip())
            if m:
                out[m.group(1)] = cp
    if not out:
        fail("aucune règle content:\"\\fXXX\" trouvée dans all.min.css — format CSS inattendu")
    return out


def page_tokens() -> tuple[set[str], set[str]]:
    """Classes icônes des 8 pages → (solid, brands). Inclut le JS embarqué
    (renderer des services : 'fa-check' par défaut, etc.). Famille : un
    préfixe explicite fab/fa-brands classe en brands ; tout le reste
    (préfixe fas, fa-, ou token nu du JS) est solid — la famille par défaut
    du portail (le renderer services écrit `class="fas " + icône`)."""
    all_tokens: set[str] = set()
    solid_prefixed: set[str] = set()
    brand_prefixed: set[str] = set()
    for page in sorted(PAGES_DIR.glob("*.html")):
        text = page.read_text(encoding="utf-8")
        all_tokens.update(TOKEN_RE.findall(text))
        solid_prefixed.update(SOLID_PREFIX_RE.findall(text))
        brand_prefixed.update(BRAND_RE.findall(text))
    solid = (all_tokens - brand_prefixed) | solid_prefixed
    brands = brand_prefixed - solid_prefixed
    # Les tokens utilitaires ne sont pas des icônes — sortis des deux jeux.
    solid -= UTILITIES
    brands -= UTILITIES
    return solid, brands


def whitelist_tokens() -> set[str]:
    src = WHITELIST_GO.read_text(encoding="utf-8")
    found = set(WHITELIST_RE.findall(src))
    if not found:
        fail(f"aucune icône extraite de {WHITELIST_GO} — la whitelist a déménagé ?")
    return found


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def ensure_full(args) -> tuple[Path, Path]:
    if args.solid and args.brands:
        return Path(args.solid), Path(args.brands)
    CACHE.mkdir(parents=True, exist_ok=True)
    solid = CACHE / f"fa-solid-900-{FA_VERSION}.woff2"
    brands = CACHE / f"fa-brands-400-{FA_VERSION}.woff2"
    if args.download or not solid.exists():
        urllib.request.urlretrieve(
            f"{CDN}/{FA_VERSION}/webfonts/fa-solid-900.woff2", solid)
    if args.download or not brands.exists():
        urllib.request.urlretrieve(
            f"{CDN}/{FA_VERSION}/webfonts/fa-brands-400.woff2", brands)
    return solid, brands


def do_subset(src: Path, dst: Path, codepoints: set[int]) -> None:
    font = TTFont(str(src))
    opts = subset.Options()
    opts.flavor = "woff2"
    opts.layout_features = []  # icônes : aucune feature OpenType requise
    opts.name_IDs = [1, 2]     # famille + sous-famille suffisent (taille)
    opts.name_languages = ["en"]
    sub = subset.Subsetter(options=opts)
    sub.populate(unicodes=list(codepoints))
    sub.subset(font)
    font.flavor = "woff2"
    font.save(str(dst))


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--solid", help="chemin du fa-solid-900.woff2 complet")
    ap.add_argument("--brands", help="chemin du fa-brands-400.woff2 complet")
    ap.add_argument("--download", action="store_true",
                    help="(re)télécharger les polices complètes depuis cdnjs")
    args = ap.parse_args()

    classmap = css_class_to_codepoint()
    page_solid, page_brands = page_tokens()
    whitelist = whitelist_tokens()

    # Résolution classe → point de code (typo = échec immédiat).
    def resolve(tokens: set[str], what: str) -> set[int]:
        pts: set[int] = set()
        for tok in sorted(tokens):
            if tok not in classmap:
                fail(f"{what} : classe « {tok} » absente de all.min.css (typo ?)")
            pts.add(classmap[tok])
        return pts

    solid_pts = resolve(page_solid, "pages (solid)") | resolve(whitelist, "whitelist")
    brands_pts = resolve(page_brands, "pages (brands)")

    solid_full, brands_full = ensure_full(args)
    sf = TTFont(str(solid_full))
    bf = TTFont(str(brands_full))
    solid_cmap, brands_cmap = sf.getBestCmap(), bf.getBestCmap()

    # Le point de code doit exister dans la police complète AVANT sous-ensemblage.
    missing = [hex(p) for p in solid_pts if p not in solid_cmap]
    if missing:
        fail(f"points de code introuvables dans fa-solid complet : {missing} "
             f"(désalignement version CSS {FA_VERSION} vs police ?)")
    missing = [hex(p) for p in brands_pts if p not in brands_cmap]
    if missing:
        fail(f"points de code introuvables dans fa-brands complet : {missing}")

    old_solid = OUT_SOLID.stat().st_size if OUT_SOLID.exists() else 0
    old_brands = OUT_BRANDS.stat().st_size if OUT_BRANDS.exists() else 0
    do_subset(solid_full, OUT_SOLID, solid_pts)
    do_subset(brands_full, OUT_BRANDS, brands_pts)

    # AUTO-VÉRIFICATION : ré-ouverture des sorties, cmap complet.
    for path, pts, name in ((OUT_SOLID, solid_pts, "solid"),
                            (OUT_BRANDS, brands_pts, "brands")):
        got = set(TTFont(str(path)).getBestCmap())
        lack = pts - got
        if lack:
            fail(f"{name} : glyphes manquants après sous-ensemblage : "
                 f"{[hex(p) for p in sorted(lack)]}")

    manifest = {
        "source": f"Font Awesome Free {FA_VERSION} (cdnjs) — ops/portal/fa-subset.py",
        "fonts": {
            "webfonts/fa-solid-900.woff2": {
                "sha256": sha256(OUT_SOLID),
                "glyphs": sorted(f"{p:04x}" for p in solid_pts),
            },
            "webfonts/fa-brands-400.woff2": {
                "sha256": sha256(OUT_BRANDS),
                "glyphs": sorted(f"{p:04x}" for p in brands_pts),
            },
        },
    }
    MANIFEST.write_text(
        json.dumps(manifest, indent=1, ensure_ascii=False) + "\n", encoding="utf-8")

    ns, nb = len(solid_pts), len(brands_pts)
    print(f"fa-solid  : {ns:3d} glyphes — {old_solid/1024:.1f} Ko → "
          f"{OUT_SOLID.stat().st_size/1024:.1f} Ko")
    print(f"fa-brands : {nb:3d} glyphe(s) — {old_brands/1024:.1f} Ko → "
          f"{OUT_BRANDS.stat().st_size/1024:.1f} Ko")
    print(f"whitelist : {len(whitelist)} icônes couvertes "
          f"({len(whitelist - page_solid)} propres à la whitelist)")
    print(f"manifeste : {MANIFEST.relative_to(ROOT)}")
    print("OK — lancer : cd backend && go test ./internal/hotpage/ "
          "(-run TestPortalWebfontsCoverIcons)")


if __name__ == "__main__":
    main()
