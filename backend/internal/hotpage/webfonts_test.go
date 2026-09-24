// webfonts_test.go — N°187 — test gardien des polices Font Awesome du
// portail captif : CHAQUE classe fa-* référencée par les 8 pages du template
// ET chaque icône de la whitelist PortalServiceIcons doit avoir son glyphe
// dans la police sous-ensemblée embarquée, et les octets embarqués doivent
// être EXACTEMENT ceux décrits par le manifeste webfonts_glyphs.json (sha256).
//
// ORIGINE (incident « icônes de services invisibles ») : N°75 a sous-ensemblé
// les polices aux 37 icônes des pages d'alors ; N°137 a ajouté la whitelist
// de 20 icônes console sans régénérer la police — le CSS contenait toutes
// les classes, mais 14 glyphes manquaient dans le woff2 : fa-money-bill-wave,
// fa-phone, fa-print, fa-store… rendaient des cases à icône VIDES sur les
// portails. Rien ne reliait la whitelist (api) à la police (hotpage) : ce
// test EST ce lien, et la whitelist a déménagé ici même (serviceicons.go)
// pour qu'il ne puisse plus être rompu.
//
// En cas d'échec : régénérer via ops/portal/fa-subset.py (procédure
// complète dans TEMPLATE.md §Webfonts) — jamais éditer le manifeste à la
// main, ni la police au doigt mouillé.
package hotpage

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"
)

//go:embed webfonts_glyphs.json
var webfontsManifestRaw []byte

// webfontManifest — miroir Go du JSON écrit par ops/portal/fa-subset.py.
type webfontManifest struct {
	Source string `json:"source"`
	Fonts  map[string]struct {
		SHA256 string   `json:"sha256"`
		Glyphs []string `json:"glyphs"`
	} `json:"fonts"`
}

// Classes utilitaires FA (animation/taille/stack) — pas des icônes, jamais
// de glyphe. Miroir de UTILITIES dans ops/portal/fa-subset.py.
var faUtilityClasses = map[string]bool{
	"fa-spin": true, "fa-spin-pulse": true, "fa-spin-reverse": true,
	"fa-pulse": true, "fa-fw": true, "fa-border": true, "fa-inverse": true,
	"fa-l": true, "fa-xs": true, "fa-sm": true, "fa-lg": true, "fa-xl": true,
	"fa-2x": true, "fa-3x": true, "fa-4x": true, "fa-5x": true,
	"fa-6x": true, "fa-7x": true, "fa-8x": true, "fa-9x": true,
	"fa-stack": true, "fa-stack-1x": true, "fa-stack-2x": true,
	"fa-sr-only": true, "fa-sr-only-focusable": true,
	"fa-flip-horizontal": true, "fa-flip-vertical": true, "fa-flip-both": true,
	"fa-rotate-90": true, "fa-rotate-180": true, "fa-rotate-270": true,
	"fa-rotate-by": true, "fa-beat": true, "fa-fade": true,
	"fa-beat-fade": true, "fa-bounce": true, "fa-shake": true, "fa-normal": true,
}

var (
	faTokenRe    = regexp.MustCompile("fa-[a-z0-9-]+")
	faSolidPreRe = regexp.MustCompile(`(?:fas|fa-solid)\s+(fa-[a-z0-9-]+)`)
	faBrandPreRe = regexp.MustCompile(`(?:fab|fa-brands)\s+(fa-[a-z0-9-]+)`)
	// Règles all.min.css : `.fa-x:before,.fa-y:before{content:"\fXXX"}` —
	// les alias FA6 partagent une règle multi-sélecteurs (piège N°75).
	faCSSRuleRe = regexp.MustCompile(`([^{}]+)\{content:"\\([0-9a-f]+)"\}`)
)

// faCSSCodepoints — classe fa-* → point de code, lu du all.min.css embarqué
// (la vérité « quelle classe existe et où pointe-t-elle », alias compris).
func faCSSCodepoints(t *testing.T) map[string]string {
	t.Helper()
	css := RawFile("css/all.min.css")
	if css == "" {
		t.Fatal("css/all.min.css absent du template embarqué")
	}
	out := map[string]string{}
	for _, m := range faCSSRuleRe.FindAllStringSubmatch(css, -1) {
		for _, sel := range strings.Split(m[1], ",") {
			sel = strings.TrimSpace(sel)
			if !strings.HasPrefix(sel, ".fa-") || !strings.HasSuffix(sel, ":before") {
				continue
			}
			cls := strings.TrimSuffix(strings.TrimPrefix(sel, "."), ":before")
			out[cls] = m[2]
		}
	}
	if len(out) < 1000 {
		t.Fatalf("parseur CSS suspect : %d classes résolues (FA6 en compte ~2000)", len(out))
	}
	return out
}

// TestPortalWebfontsCoverIcons — le contrat complet : pages + whitelist
// couvertes par les glyphes du manifeste, manifeste = polices embarquées.
func TestPortalWebfontsCoverIcons(t *testing.T) {
	var mf webfontManifest
	if err := json.Unmarshal(webfontsManifestRaw, &mf); err != nil {
		t.Fatalf("manifeste webfonts_glyphs.json illisible : %v", err)
	}
	solidEntry, okSolid := mf.Fonts["webfonts/fa-solid-900.woff2"]
	brandsEntry, okBrands := mf.Fonts["webfonts/fa-brands-400.woff2"]
	if !okSolid || !okBrands || len(mf.Fonts) != 2 {
		t.Fatalf("manifeste inattendu : attendu exactement fa-solid-900 + fa-brands-400, eu %d entrées", len(mf.Fonts))
	}
	solidGlyphs := map[string]bool{}
	for _, g := range solidEntry.Glyphs {
		solidGlyphs[g] = true
	}
	brandsGlyphs := map[string]bool{}
	for _, g := range brandsEntry.Glyphs {
		brandsGlyphs[g] = true
	}

	// 1. Les octets embarqués SONT ceux du manifeste (sha256) — une police
	// remplacée sans repasser le script casse ici, pas en production.
	for path, entry := range mf.Fonts {
		b, ok := File(path)
		if !ok {
			t.Fatalf("%s déclaré au manifeste mais absent du template", path)
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != entry.SHA256 {
			t.Fatalf("%s embarqué ≠ manifeste (sha256 %s ≠ %s) — régénérer via ops/portal/fa-subset.py",
				path, got, entry.SHA256)
		}
	}

	cssMap := faCSSCodepoints(t)

	// 2. La whitelist « Nos Services » est ENTIÈREMENT dans la police solid
	// (le renderer écrit `class="fas " + icône` — famille solid, jamais brands).
	var wl []string
	for icon := range PortalServiceIcons {
		wl = append(wl, icon)
	}
	sort.Strings(wl)
	for _, icon := range wl {
		cp, ok := cssMap[icon]
		if !ok {
			t.Fatalf("whitelist : classe %q absente de all.min.css — classe inconnue du CSS embarqué", icon)
		}
		if !solidGlyphs[cp] {
			t.Errorf("whitelist : %q (U+%s) n'a pas de glyphe dans fa-solid-900.woff2 — "+
				"l'icône s'afficherait VIDE sur le portail — régénérer la police via ops/portal/fa-subset.py", icon, cp)
		}
	}

	// 3. Chaque classe fa-* référencée par les pages (HTML + JS embarqué)
	// est couverte — une icône ajoutée à une page sans régénérer la police
	// (le piège N°137, côté template cette fois) casse ici.
	pages, err := fs.Glob(templateFS, "template/*.html")
	if err != nil || len(pages) == 0 {
		t.Fatalf("aucune page HTML dans le template embarqué (err=%v)", err)
	}
	for _, p := range pages {
		b, err := fs.ReadFile(templateFS, p)
		if err != nil {
			t.Fatalf("lecture %s : %v", p, err)
		}
		text := string(b)
		solidPrefixed := map[string]bool{}
		for _, m := range faSolidPreRe.FindAllStringSubmatch(text, -1) {
			solidPrefixed[m[1]] = true
		}
		brandPrefixed := map[string]bool{}
		for _, m := range faBrandPreRe.FindAllStringSubmatch(text, -1) {
			brandPrefixed[m[1]] = true
		}
		for _, tok := range faTokenRe.FindAllString(text, -1) {
			if faUtilityClasses[tok] {
				continue
			}
			cp, ok := cssMap[tok]
			if !ok {
				t.Errorf("%s : classe %q inconnue de all.min.css (typo ? classe utilitaire non listée ?)",
					strings.TrimPrefix(p, "template/"), tok)
				continue
			}
			name := strings.TrimPrefix(p, "template/")
			switch {
			case brandPrefixed[tok] && !solidPrefixed[tok]:
				if !brandsGlyphs[cp] && !solidGlyphs[cp] {
					t.Errorf("%s : %q (U+%s, brands) sans glyphe dans aucune police embarquée — régénérer via ops/portal/fa-subset.py",
						name, tok, cp)
				}
			default: // solid : préfixe fas explicite ou token nu (JS renderer)
				if !solidGlyphs[cp] && !brandsGlyphs[cp] {
					t.Errorf("%s : %q (U+%s) sans glyphe dans aucune police embarquée — régénérer via ops/portal/fa-subset.py",
						name, tok, cp)
				}
			}
		}
	}

	if len(solidEntry.Glyphs) < len(wl) {
		t.Fatalf("police solid vide ou ridicule (%d glyphes) — le sous-ensemblage a raté", len(solidEntry.Glyphs))
	}
	fmt.Printf("webfonts gardien OK : solid %d glyphes, brands %d, whitelist %d icônes, pages %d couvertes\n",
		len(solidEntry.Glyphs), len(brandsEntry.Glyphs), len(wl), len(pages))
}
