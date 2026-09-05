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
├── img/              ← logo.png · pub1.jpg · pub2.jpg · pub3.jpg (carrousel Swiper)
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
| Logo | `img/logo.png` (+ fallback texte « SC » si absent) |
| Messages animés | tableau `strings` de `new Typed(...)` dans login.html |
| Support WhatsApp / pied de page | lien `wa.me` et `.ftci-link` dans login.html |
| Messages d'erreur | `errors.txt` (syntaxe `$(error-orig)` etc.) |

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
