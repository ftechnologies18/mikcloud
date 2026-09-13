// Walled garden — liste blanche de domaines accessible avant authentification.
// Extrait du monolithe agent.go (N°88) — même package, contenu inchangé.
package agent

import (
	"mikcloud/hotspot-api/internal/model"
	"strconv"
	"strings"
)

// WalledGardenMarker — commentaire des règles walled-garden posées par
// MikCloud (N°29 — runbook N°27-D automatisé). L'idempotence s'appuie dessus :
// seules les règles portant ce marqueur sont remplacées — les règles
// personnelles du gérant sont préservées.
const WalledGardenMarker = "mikcloud-wg"

// SafeWifiMarker — commentaire des règles NAT posées par SafeWiFi (N°80 —
// protection DNS du WiFi public). Même contrat d'idempotence : seules les
// règles portant ce marqueur sont retirées puis recréées — les règles du
// gérant sont préservées.

// ---------------------------------------------------------------------------
// N°29 — walled-garden d'inscription publique (runbook N°27-D automatisé)
// ---------------------------------------------------------------------------

// SanitizeWGDomain — hôte walled-garden sûr : minuscules, [a-z0-9._-] plus un
// suffixe de port NUMÉRIQUE (déploiements non standard), 253 caractères max.
// Tout le reste est refusé : ces valeurs sont injectées dans un script
// RouterOS (défense en profondeur, rosEscape reste appliqué à l'écriture).
func SanitizeWGDomain(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || len(s) > 253 {
		return ""
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == ':') {
			return ""
		}
	}
	if i := strings.IndexByte(s, ':'); i >= 0 { // port numérique uniquement
		port := s[i+1:]
		if port == "" || len(port) > 5 {
			return ""
		}
		for j := 0; j < len(port); j++ {
			if port[j] < '0' || port[j] > '9' {
				return ""
			}
		}
	}
	return s
}

// WalledGardenDomainsFromPayload — les domaines d'une commande walled_garden
// ([]any JSON ou []string mémoire, cf. plStrList), assainis.

// WalledGardenDomainsFromPayload — les domaines d'une commande walled_garden
// ([]any JSON ou []string mémoire, cf. plStrList), assainis.
func WalledGardenDomainsFromPayload(p map[string]any) []string {
	raw := plStrList(p, "domains")
	out := make([]string, 0, len(raw))
	for _, d := range raw {
		if d = SanitizeWGDomain(d); d != "" {
			out = append(out, d)
		}
	}
	return out
}

// walledGardenInstallBlock — bloc walled-garden du script d'INSTALLATION
// (N°29) : la page d'inscription publique /join/{token} et l'API qu'elle
// appelle restent joignables AVANT authentification depuis le WiFi du hotspot
// (le scan du QR fonctionne sur place, cf. docs/RUNBOOK-WALLED-GARDEN.md).
// Idempotent : seules les règles marquées "mikcloud-wg" sont remplacées.
// Corps multi-lignes (règle du parseur console — cf. en-tête InstallScript).
// Vide si aucun domaine annoncé par le déploiement.

// walledGardenInstallBlock — bloc walled-garden du script d'INSTALLATION
// (N°29) : la page d'inscription publique /join/{token} et l'API qu'elle
// appelle restent joignables AVANT authentification depuis le WiFi du hotspot
// (le scan du QR fonctionne sur place, cf. docs/RUNBOOK-WALLED-GARDEN.md).
// Idempotent : seules les règles marquées "mikcloud-wg" sont remplacées.
// Corps multi-lignes (règle du parseur console — cf. en-tête InstallScript).
// Vide si aucun domaine annoncé par le déploiement.
func walledGardenInstallBlock(domains []string) string {
	if len(domains) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n    # Walled-garden d'inscription publique (N°27/N°29) : la page /join et\n")
	sb.WriteString("    # son API restent joignables AVANT authentification — le scan du QR\n")
	sb.WriteString("    # fonctionne depuis le WiFi du hotspot. Seules les règles marquées\n")
	sb.WriteString("    # \"" + WalledGardenMarker + "\" sont remplacées, les vôtres sont conservées.\n")
	sb.WriteString("    :do {\n      /ip hotspot walled-garden remove [find comment=\"" + WalledGardenMarker + " page\"]\n    } on-error={}\n")
	sb.WriteString("    :do {\n      /ip hotspot walled-garden ip remove [find comment=\"" + WalledGardenMarker + " page\"]\n    } on-error={}\n")
	sb.WriteString("    :do {\n      /ip hotspot walled-garden remove [find comment=\"" + WalledGardenMarker + " dns\"]\n    } on-error={}\n")
	for _, d := range domains {
		sb.WriteString("    :do {\n      :if ([:len [/ip hotspot walled-garden find comment=\"" + WalledGardenMarker + " page\" dst-host=\"" + rosEscape(d) + "\"]] = 0) do={ /ip hotspot walled-garden add action=allow dst-host=\"" + rosEscape(d) + "\" comment=\"" + WalledGardenMarker + " page\" }\n    } on-error={}\n")
		// N°48 — miroir ip (action=accept, cf. N°31-d) : couverture HTTPS.
		sb.WriteString("    :do {\n      :if ([:len [/ip hotspot walled-garden ip find comment=\"" + WalledGardenMarker + " page\" dst-host=\"" + rosEscape(d) + "\"]] = 0) do={ /ip hotspot walled-garden ip add action=accept dst-host=\"" + rosEscape(d) + "\" comment=\"" + WalledGardenMarker + " page\" }\n    } on-error={}\n")
	}
	// N°48 — règles « api » (variante ip, action=accept, dst-host) : la
	// variante proxy ci-dessus ne voit que le HTTP pur (port 80) — or l'API
	// (claim, /portal, /join) est en HTTPS (Render/Vercel). Sans règles ip,
	// le TLS 443 pré-auth restait bloqué par le hotspot → le fetch du claim
	// échouait côté client (« Service WiFi offert momentanément indisponible
	// »). Pas de restriction de port : couvre TCP 80/443 ET UDP 443 (QUIC).
	sb.WriteString("    :do {\n      /ip hotspot walled-garden ip remove [find comment=\"" + WalledGardenMarker + " api\"]\n    } on-error={}\n")
	for _, d := range domains {
		sb.WriteString("    :do {\n      :if ([:len [/ip hotspot walled-garden ip find comment=\"" + WalledGardenMarker + " api\" dst-host=\"" + rosEscape(d) + "\"]] = 0) do={ /ip hotspot walled-garden ip add action=accept dst-host=\"" + rosEscape(d) + "\" comment=\"" + WalledGardenMarker + " api\" }\n    } on-error={}\n")
	}
	sb.WriteString("    :do {\n      /ip hotspot walled-garden ip remove [find comment=\"" + WalledGardenMarker + " dns\"]\n    } on-error={}\n")
	sb.WriteString("    :do {\n      :if ([:len [/ip hotspot walled-garden ip find comment=\"" + WalledGardenMarker + " dns\" protocol=udp]] = 0) do={ /ip hotspot walled-garden ip add action=accept protocol=udp dst-port=53 comment=\"" + WalledGardenMarker + " dns\" }\n    } on-error={}\n")
	sb.WriteString("    :do {\n      :if ([:len [/ip hotspot walled-garden ip find comment=\"" + WalledGardenMarker + " dns\" protocol=tcp]] = 0) do={ /ip hotspot walled-garden ip add action=accept protocol=tcp dst-port=53 comment=\"" + WalledGardenMarker + " dns\" }\n    } on-error={}\n")
	return sb.String()
}

// buildWalledGarden — N°29 : applique le walled-garden d'inscription publique
// sur un routeur AGENT déjà en ligne (le script d'installation le fait pour
// les routeurs neufs). Idempotent : les règles marquées sont remplacées, les
// autres préservées. Les 2 règles DNS (udp/tcp 53) garantissent que la
// résolution traverse le routeur même pour les clients avec DNS codé en dur —
// le matching par domaine du walled-garden s'appuie sur le reniflement DNS.
// Rapport : domains = nombre de règles page/api réellement posées.
// N°32 — TRAÇAGE : une variable RouterOS « step » est posée avant chaque bloc
// à risque et embarquée dans le rapport d'erreur (concaténation console
// « . $step », construct d'expression identique au « (“…”) » prouvé) — le
// constat prod du 05/09 (3× echec_sur_le_routeur sur le script « propre »,
// alors que le même script à 5 domaines passe) exige de savoir QUELLE ligne
// échoue sans accès console au routeur client.

// buildWalledGarden — N°29 : applique le walled-garden d'inscription publique
// sur un routeur AGENT déjà en ligne (le script d'installation le fait pour
// les routeurs neufs). Idempotent : les règles marquées sont remplacées, les
// autres préservées. Les 2 règles DNS (udp/tcp 53) garantissent que la
// résolution traverse le routeur même pour les clients avec DNS codé en dur —
// le matching par domaine du walled-garden s'appuie sur le reniflement DNS.
// Rapport : domains = nombre de règles page/api réellement posées.
// N°32 — TRAÇAGE : une variable RouterOS « step » est posée avant chaque bloc
// à risque et embarquée dans le rapport d'erreur (concaténation console
// « . $step », construct d'expression identique au « (“…”) » prouvé) — le
// constat prod du 05/09 (3× echec_sur_le_routeur sur le script « propre »,
// alors que le même script à 5 domaines passe) exige de savoir QUELLE ligne
// échoue sans accès console au routeur client.
func (b Builder) buildWalledGarden(cmd model.Command) string {
	domains := WalledGardenDomainsFromPayload(cmd.Payload)
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	// N°31-c — BATTEMENT DE CŒUR : le script confirme SA LIVRAISON avant
	// d'attaquer les lignes à risque (construct « status=started », identique
	// aux fetch de rapport — prouvé 849+ fois). Si l'import meurt ensuite sur
	// une ligne que ce RouterOS rejette, le cloud sait au moins que le
	// fichier est ARRIVÉ — l'ancien silence total rendait tout diagnostic
	// impossible (constat prod : 2 livraisons sans AUCUN signal).
	sb.WriteString(`/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + urlEscape(cmd.ID) + `&status=started") output=none` + "\n")
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(`:local step "start"` + "\n")
	step := func(s string) { sb.WriteString(`:set step "` + s + `"` + "\n") }
	// N°31-c — find EXACT (même classe syntaxique que `find name="..."` des
	// user_remove — prouvé 4×) au lieu du regex `comment~"..."` — suspect
	// n°1 du blocage d'import constaté en prod (chunk muet 2×/2×, commandes
	// du même check-in tuées avec lui). Nos règles portent EXACTEMENT les
	// commentaires ci-dessous : la suppression exacte est complète.
	// N°31-e — removes SILENCIEUX (best-effort) : une règle « en usage » par
	// les clients du hotspot (flux DNS permanents sur les règles DNS — constat
	// prod 18:26→18:30 : 6 re-filés error d’affilée) ne doit PAS faire
	// échouer la mise à jour : le service prime sur le ménage.
	step("rm-page")
	sb.WriteString(":do { /ip hotspot walled-garden remove [find comment=\"" + WalledGardenMarker + " page\"] } on-error={}\n")
	step("rm-api-ip")
	sb.WriteString(":do { /ip hotspot walled-garden ip remove [find comment=\"" + WalledGardenMarker + " api\"] } on-error={}\n")
	step("rm-dns-ip")
	sb.WriteString(":do { /ip hotspot walled-garden ip remove [find comment=\"" + WalledGardenMarker + " dns\"] } on-error={}\n")
	// N°31-e — adds CONDITIONNELS à l’absence : si le remove vient d’échouer,
	// la règle existe DÉJÀ (service assuré) → skip — PAS de doublon, PAS
	// d’erreur. Seule une vraie erreur d’add met okVar à false.
	// N°32 — chaque bloc à risque est précédé de :set step : le rapport
	// d’erreur embarque la ligne fautive (« &step=" . $step ») — diagnostic
	// sans accès console (les removes étant best-effort, seuls les adds
	// peuvent porter okVar à false).
	// N°31-d — action=ACCEPT (et NON allow) sur walled-garden ip : la table
	// n’accepte que accept|drop|reject (doc officielle HotSpot) — « allow »
	// est une erreur de validation console qui rejetait TOUT le fichier
	// d’import (constat prod : 4 livraisons muettes, rien ne s’exécutait).
	for i, d := range domains {
		step("add-page-" + strconv.Itoa(i+1))
		sb.WriteString(":do { :if ([:len [/ip hotspot walled-garden find comment=\"" + WalledGardenMarker + " page\" dst-host=\"" + rosEscape(d) + "\"]] = 0) do={ /ip hotspot walled-garden add action=allow dst-host=\"" + rosEscape(d) + "\" comment=\"" + WalledGardenMarker + " page\" } } on-error={ :set " + okVar + " false }\n")
	}
	// N°48 — règles « api » (variante ip) : le HTTPS pré-auth passe ICI, pas
	// dans la variante proxy (page) qui ne voit que le HTTP pur. La variante
	// ip n'accepte que accept|drop|reject (N°31-d) → action=accept ; le
	// matching dst-host s'appuie sur le reniflement DNS (les règles DNS
	// ci-dessous garantissent que la résolution transite par le routeur,
	// même pour les clients avec DNS codé en dur).
	for i, d := range domains {
		step("add-api-" + strconv.Itoa(i+1))
		sb.WriteString(":do { :if ([:len [/ip hotspot walled-garden ip find comment=\"" + WalledGardenMarker + " api\" dst-host=\"" + rosEscape(d) + "\"]] = 0) do={ /ip hotspot walled-garden ip add action=accept dst-host=\"" + rosEscape(d) + "\" comment=\"" + WalledGardenMarker + " api\" } } on-error={ :set " + okVar + " false }\n")
	}
	step("add-dns-udp")
	sb.WriteString(":do { :if ([:len [/ip hotspot walled-garden ip find comment=\"" + WalledGardenMarker + " dns\" protocol=udp]] = 0) do={ /ip hotspot walled-garden ip add action=accept protocol=udp dst-port=53 comment=\"" + WalledGardenMarker + " dns\" } } on-error={ :set " + okVar + " false }\n")
	step("add-dns-tcp")
	sb.WriteString(":do { :if ([:len [/ip hotspot walled-garden ip find comment=\"" + WalledGardenMarker + " dns\" protocol=tcp]] = 0) do={ /ip hotspot walled-garden ip add action=accept protocol=tcp dst-port=53 comment=\"" + WalledGardenMarker + " dns\" } } on-error={ :set " + okVar + " false }\n")
	ok := b.reportLine(cmd.ID, true, map[string]string{"domains": strconv.Itoa(len(domains))})
	ko := `/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + urlEscape(cmd.ID) + `&status=error&message=echec_sur_le_routeur&step=" . $step) output=none`
	sb.WriteString(":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n")
	return sb.String()
}

// HotspotFilesFromPayload — la liste ordonnée des fichiers à déployer pour
// une commande hotspot_files. Chaque entrée porte le chemin RELATIF
// (login.html, status.html, css/bootstrap.min.css, …) qui sert à la fois de
// clé de signature et de dst-path côté routeur (sous le dossier hotspot/).
