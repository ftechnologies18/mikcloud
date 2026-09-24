# Hotspot Page — Portail captif MikroTik (CYBER ESPACE SC)

Page de connexion du hotspot MikroTik (design « MNASPOT MODERN » — glassmorphism,
thème teal/ambre), personnalisée FTCI. Ces fichiers sont déployés dans le
**html-directory** du hotspot du routeur : RouterOS les sert tels quels sur le
réseau captif, **avant toute authentification**.

> ⚠️ **Noms de fichiers = contrat RouterOS.** Les pages listées ci-dessous sont
> appelées par le routeur par leur nom exact (`/login`, `/status`, …). Ne ni
> renommer, ni déplacer ces fichiers. Seuls `css/`, `js/`, `img/`, `webfonts/`
> sont des dossiers d'assets libres (tant que les références restent cohérentes).

## Structure

```
Hotspot Page/
├── login.html        ← PAGE PRINCIPALE — connexion Ticket / Membre + offres Wave
├── alogin.html       ← redirection après login ($(link-status))
├── status.html       ← statut de session (IP, quota, temps restant, auto-refresh)
├── logout.html       ← confirmation de déconnexion
├── error.html        ← erreurs d'authentification ($(error-orig))
├── redirect.html     ← page de redirection ($(link-redirect))
├── rlogin.html       ← re-login automatique (requis par l'advert / session expirée)
├── radvert.html      ← page « publicité requise » (try-again vers $(link-orig))
├── errors.txt        ← messages d'erreur RouterOS (personnalisables)
├── errors-en.txt     ← copie anglaise de référence (RouterOS ne lit que errors.txt)
├── md5.js            ← hash CHAP (exigé par $(if chap-id) dans login.html)
├── favicon.ico       ← servi à la racine du portail
├── css/              ← bootstrap.min.css · all.min.css (Font Awesome) · swiper-bundle.min.css
├── js/               ← typed.umd.js · swiper-bundle.min.js
├── img/              ← pub1.jpg · pub2.jpg · pub3.jpg (carrousel Swiper — N°135 : logo.png RETIRÉ, le logo vient du cloud)
└── webfonts/         ← polices Font Awesome (référencées par css/all.min.css : ../webfonts/)
```

## Déploiement sur le routeur

1. **FTP** (recommandé, préserve l'arborescence) :
   ```
   ftp <IP_DU_ROUTEUR>      # utilisateur admin
   cd hotspot                # html-directory configurée dans /ip hotspot profile
   put login.html … (ou mput + mkdir css/js/img/webfonts)
   ```
2. **Winbox** : glisser-déposer le contenu du dossier (sans ce README) dans
   `Files` → dossier du hotspot.
3. Vérifier : `/ip hotspot profile` → `html-directory` pointe bien sur ce dossier.

> Éditer/tester en local : ouvrir `login.html` directement dans un navigateur
> fonctionne (les variables `$(…)` restent affichées telles quelles) ; tester le
> CHAP et la vraie connexion exige le routeur.

## Intégration MikCloud

- **Inscriptions publiques** : la page de login peut rediriger vers le portail
  d'inscription `/join/{token}?mac=$(mac-esc)` servi par MikCloud (anti-abus par
  MAC — cf. CHANGELOG N°33 et `docs/RUNBOOK-WALLED-GARDEN.md`).
- **Walled-garden** : les domaines appelés **avant authentification** doivent
  être acceptés (`/ip hotspot walled-garden ip`) — scanner QR
  (`laksa19.github.io`), éventuellement l'API MikCloud. Voir
  [`docs/RUNBOOK-WALLED-GARDEN.md`](../docs/RUNBOOK-WALLED-GARDEN.md).
- **Paiements Wave** : les cartes d'offres pointent vers des liens
  `pay.wave.com` (deep-link intent Android géré dans login.html). Le portail
  **kiosque** MikCloud (45 s → redirection `generate_204`) ramène ensuite vers
  cette page de login.

## Personnalisation courante

| Quoi | Où |
|---|---|
| Couleurs / thème | variables `:root` dans login.html (`--primary`, `--bg-1…`) |
| Offres & prix (liens Wave) | cartes `.creative-card` / `.card-featured` dans login.html |
| Carrousel promo | remplacer `img/pub1..3.jpg` (mêmes noms) |
| Logo | `{{MIKCLOUD_LOGO_BLOCK}}` — img du logo DU CLIENT (`logoUrl` console) ou repli initiale du tenant (N°135 : `img/logo.png` retiré — jamais le logo d'un autre client) |
| Services « Nos Services » | `{{MIKCLOUD_SERVICES_ATTR}}` + `{{MIKCLOUD_SERVICES_BLOCK}}` — services DU TENANT (`portalServices` console, ≤ 6, icônes FA curées) ou section masquée (N°137 : les 4 services historiques du pilote sont chassés du template) |
| Messages animés | `{{MIKCLOUD_TICKER_JSON}}` — messages DU TENANT (`portalTicker` console, ≤ 5, 80 car.) ou repli des 3 messages historiques (N°138 : le serveur substitue le tableau dans l'init Typed.js) |
| Support WhatsApp / pied de page | `{{MIKCLOUD_WHATSAPP_HREF}}` + `{{MIKCLOUD_WHATSAPP_LABEL}}` — numéro DU TENANT (`portalWhatsapp` console, chiffres 8-15 + libellé ≤ 30 car.) ou repli support MikCloud, posés dans login/logout/error (N°139) ; le crédit `.ftci-link` reste serve (éditeur) |
| Messages d'erreur | `errors.txt` (syntaxe `$(error-orig)` etc.) |

## Webfonts Font Awesome (sous-ensemblage — N°75/N°187)

`webfonts/fa-solid-900.woff2` + `fa-brands-400.woff2` sont des SOUS-ENSEMBLES
(~5 Ko + 0,5 Ko vs 150 Ko + 108 Ko complets) : le flash MikroTik est compté,
le portail n'a besoin que des icônes réellement affichables.

**Ce que le sous-ensemble DOIT couvrir (les deux sources de vérité) :**

1. chaque classe `fa-*` référencée par les 8 pages (HTML + JS embarqué),
   famille comprise (`fab` → brands, le reste → solid) ;
2. la whitelist des icônes « Nos Services » (`hotpage.PortalServiceIcons`,
   `serviceicons.go`) — le renderer services écrit `class="fas " + icône`.

**Régénérer après toute addition** (icône dans une page, whitelist étendue) :

```
pip install fonttools brotli
python3 ops/portal/fa-subset.py            # cache ~/.cache/mikcloud-fa/
python3 ops/portal/fa-subset.py --download # si le cache est vide
```

Le script écrit les deux woff2 + le manifeste `webfonts_glyphs.json` (sha256 +
glyphes, DANS le paquet Go, jamais déployé aux routeurs). Le test gardien
`hotpage.TestPortalWebfontsCoverIcons` casse la CI si pages/whitelist et
police divergent, ou si le manifeste ne décrit plus les octets embarqués.

> Incident d'origine (N°187) : la whitelist N°137 (20 icônes) avait été
> ajoutée sans régénérer la police sous-ensemblée N°75 (37 glyphes) — 14
> icônes sans glyphe, dont `fa-money-bill-wave`/`fa-phone`/`fa-print`/
> `fa-store` utilisés en production : cases à icônes VIDES sur le portail.
> La version FA doit rester 6.4.0, appairée au `css/all.min.css` embarqué
> (les points de code dépendent de la version).

## Historique du nettoyage (N°34)

Le commit d'origine (`913b151`) contenait le template Mikhmon brut. Ont été
retirés (récupérables via `git show 913b151:'<chemin>'`) :

- `debug.log` (log Windows parasite), `euh.html` (brouillon « Mnaspot ») ;
- `engine1/` + `data1/` (slider WOW Slider généré, non référencé — ~290 Ko) ;
- `css/style.css`, `css/mikhmon-ui-light.css`, `css/background.css` (non liés) ;
- `js/jquery-3.2.1.min.js` (n'exigeait que WOW Slider), `js/typed.min.js`
  (doublon de `typed.umd.js`) ;
- `img/bg-body.png`, `img/favicon.png` (non référencés) ;
- `login.html` : script `js/bootstrap.bundle.min.js` retiré (fichier absent du
  dossier → 404 sur le routeur ; aucun composant Bootstrap JS utilisé).

Gain : ~800 Ko de flash routeur et zéro requête 404 côté client.
