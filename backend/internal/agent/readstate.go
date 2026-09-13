// Lecture d'état routeur : watcher, read-state paginé, DHCP, hosts, cookies, logs, scheduler, ressources.
// Extrait du monolithe agent.go (N°88) — même package, contenu inchangé.
package agent

import (
	"mikcloud/hotspot-api/internal/model"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Builders par kind
// ---------------------------------------------------------------------------

// ReadChunkSize — N°76 — nombre d'utilisateurs hotspot rapportés par commande
// read_state. La réconciliation des grands parcs est PAGINÉE : le script
// rapporte une fenêtre [start, start+count) du parc + le total exact, et le
// cloud enchaîne les chunks jusqu'au rapport complet (pattern import_hotspot,
// éprouvé en production). La taille garde le corps POST (http-data) loin de la
// limite RouterOS (~64 Ko) : 500 entrées « name|profile|disabled; » ≈ 20 Ko.
const ReadChunkSize = 500

// MaxReadChunks — borne ABSOLUE de chunks par cycle read_state (20 × 500 =
// 10 000 users). Au-delà, le cloud n'enchaîne plus : le rapport ne sera jamais
// complet, il reste honnête (aucune déduction sur les absents — comportement
// trunc N°75) plutôt que de mentir sur un parc hors d'atteinte du protocole.

// MaxReadChunks — borne ABSOLUE de chunks par cycle read_state (20 × 500 =
// 10 000 users). Au-delà, le cloud n'enchaîne plus : le rapport ne sera jamais
// complet, il reste honnête (aucune déduction sur les absents — comportement
// trunc N°75) plutôt que de mentir sur un parc hors d'atteinte du protocole.
const MaxReadChunks = 20

// watcherOnEvent — N°77 — corps UNE LIGNE (séparateurs « ; », formes valides
// en import .rsc — même sérialisation que buildSchedulerAdd) du on-event du
// veilleur d'invités. Reçoit l'URL et le token DÉJÀ échappés pour le niveau de
// citation INTERNE (le corps contient ses propres chaînes quotées) ; l'appelant
// ré-échappe le corps entier pour le niveau on-event="…" (double échappement
// assumé et correct : chaque niveau de citation décode le sien).
//
// Sémantique : à chaque tick (20 s), compter les hôtes hotspot NON autorisés —
// un hôte non autorisé = un appareil connecté qui n'a PAS encore de session :
// c'est exactement la fenêtre « invité sur le portail, claim imminent ou en
// cours ». Si > 0 → check-in complet (fichier PROPRE au veilleur, jamais le
// dst-path du scheduler principal : deux fetchs concurrents ne peuvent pas
// s'écraser le fichier). Si 0 → RIEN (aucun octet émis — la veille N°75 garde
// ses 6 Mo/mois). Exclusions : bypassed (binding MAC permanent — sinon le
// veilleur tirerait 24 h/24 pour un appareil du gérant) et blocked (banni du
// login : aucun claim ne viendra de lui).

// watcherOnEvent — N°77 — corps UNE LIGNE (séparateurs « ; », formes valides
// en import .rsc — même sérialisation que buildSchedulerAdd) du on-event du
// veilleur d'invités. Reçoit l'URL et le token DÉJÀ échappés pour le niveau de
// citation INTERNE (le corps contient ses propres chaînes quotées) ; l'appelant
// ré-échappe le corps entier pour le niveau on-event="…" (double échappement
// assumé et correct : chaque niveau de citation décode le sien).
//
// Sémantique : à chaque tick (20 s), compter les hôtes hotspot NON autorisés —
// un hôte non autorisé = un appareil connecté qui n'a PAS encore de session :
// c'est exactement la fenêtre « invité sur le portail, claim imminent ou en
// cours ». Si > 0 → check-in complet (fichier PROPRE au veilleur, jamais le
// dst-path du scheduler principal : deux fetchs concurrents ne peuvent pas
// s'écraser le fichier). Si 0 → RIEN (aucun octet émis — la veille N°75 garde
// ses 6 Mo/mois). Exclusions : bypassed (binding MAC permanent — sinon le
// veilleur tirerait 24 h/24 pour un appareil du gérant) et blocked (banni du
// login : aucun claim ne viendra de lui).
func watcherOnEvent(urlEsc, tokEsc string) string {
	return ":do { " +
		":local mkgw 0; " +
		":do { :set mkgw [/ip hotspot host print count-only where !authorized && !bypassed && !blocked] } on-error={ :set mkgw 0 }; " +
		":if ($mkgw > 0) do={ " +
		":local mkwf \"yes\"; " +
		":do { /tool fetch url=\"" + urlEsc + "/agent/cmd?token=" + tokEsc + "\" dst-path=\"" + WatcherFilename + "\" } on-error={ :set mkwf \"no\" }; " +
		":if ($mkwf = \"yes\") do={ :delay 2s; /import file-name=\"" + WatcherFilename + "\" } " +
		"} " +
		"} on-error={}"
}

// buildWatcherEnsure — N°77 — déploie (ou redéploie) le veilleur d'invités sur
// un routeur agent : remove-then-add idempotent, rapporté comme toute commande.
// Servi aux routeurs dont WatcherOK est faux (ensureWatcherLocked au check-in —
// pattern walled_garden : convergence automatique du parc existant en UN
// check-in, re-file tant que le retour « ok » n'est pas arrivé).

// buildWatcherEnsure — N°77 — déploie (ou redéploie) le veilleur d'invités sur
// un routeur agent : remove-then-add idempotent, rapporté comme toute commande.
// Servi aux routeurs dont WatcherOK est faux (ensureWatcherLocked au check-in —
// pattern walled_garden : convergence automatique du parc existant en UN
// check-in, re-file tant que le retour « ok » n'est pas arrivé).
func (b Builder) buildWatcherEnsure(cmd model.Command) string {
	urlEsc := rosEscape(strings.TrimRight(b.BaseURL, "/"))
	tokEsc := rosEscape(b.Token)
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do {\n  /system scheduler remove [find name=\"" + WatcherName + "\"]\n} on-error={}\n")
	sb.WriteString(":do {\n  /system scheduler add name=\"" + WatcherName + "\" interval=" + strconv.Itoa(WatcherIntervalSec) + "s start-time=startup on-event=\"" +
		rosEscape(watcherOnEvent(urlEsc, tokEsc)) + "\"\n} on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

// buildSafeWifi — N°80 : protection DNS du WiFi public (SafeWiFi) —
// durcie N°85 : le DNS filtré devient le SEUL chemin de sortie.
//
// Principe : deux règles NAT dst-nat réécrivent TOUT le port 53 transitant
// par le routeur (requêtes des clients vers n'importe quel résolveur, y
// compris celles adressées au routeur lui-même) vers le résolveur filtrant
// du niveau. Le DNS PROPRE du routeur n'est pas touché (son trafic part en
// chain=output, hors dstnat) : le check-in agent et la résolution locale
// restent intacts quel que soit l'état du résolveur filtrant — la
// disponibilité du site prime sur la stricteté du filtrage, et /ip dns
// n'est JAMAIS modifié.
//
// Durcissement N°85 — le constat terrain qui l'impose : un site adulte
// restait accessible sur un routeur « Protection familles » active. Trois
// échappatoires à fermer, une par une :
//
//  1. ORDRE DES RÈGLES — les règles NAT se posaient en FIN de table : une
//     règle dstnat antérieure (redirect DNS hérité d'une config Mikhmon ou
//     d'un tutoriel hotspot) interceptait le port 53 AVANT MikCloud,
//     silencieusement — la signature ne comptait que la PRÉSENCE des
//     règles marquées (rules=2), jamais leur EFFECTIVITÉ. Désormais
//     place-before=0 : tête de table, rien ne passe devant (miroir Shield
//     N°81 / FamilyGuard N°82). Bump du sel sw-v1 → sw-v2 : tout le parc
//     reçoit la nouvelle forme au check-in suivant, sans intervention.
//
//  2. DNS CHIFFRÉ — DoT (tcp/853, « DNS privé » Android) est bloqué depuis
//     l'interface hotspot ; DoH (tcp/443 vers les endpoints publics connus,
//     liste SafeWifiDoHList) est bloqué de même : l'appareil retombe sur le
//     DNS simple → redirigé → filtré. Limite résiduelle documentée : un
//     endpoint DoH exotique hors liste reste joignable, un VPN contourne
//     toute protection DNS (DPI hors de portée d'un routeur 128 Mo).
//
//  3. IPv6 — le NAT SafeWiFi est IPv4 : un appareil dual-stack résolvait
//     en IPv6, hors de portée des règles. Désormais DNS IPv6 (tcp+udp 53),
//     DoT et DoH IPv6 sont coupés depuis l'interface hotspot : repli IPv4,
//     redirigé → filtré. Best-effort (on-error silencieux) : un routeur
//     sans pile IPv6 n'a ni règles à poser ni échappatoire à fermer.
//
//     off      : retire les règles et listes marquées (retour à l'état antérieur) ;
//     threats  : Quad9 (malwares, phishing, arnaques) ;
//     family   : AdGuard Family (+ contenus adultes, publicités).
//
// Idempotent : seuls les objets marqués "mikcloud-safewifi" sont remplacés
// (NAT + FILTER v4, listes DoH v4/v6, règles IPv6), ceux du gérant sont
// conservés. Le rapport échoe le compte d'objets marqués IPv4 présents
// APRÈS application (nat + filter + address-list — vérité routeur : le
// cloud ne pose la signature que si ce compte est exact) ET le nombre de
// serveurs hotspots trouvés (pattern Shield N°81 : le compte attendu en
// dépend).

// buildReadState — v5 (N°76) : télémétrie + rapport PAGINÉ des utilisateurs.
// Motivation : v4 (N°75) bornait le rapport à 500 users et gelait TOUTE
// déduction au-delà (badge « absent du routeur » ni posé ni levé) — les faux
// badges posés avant N°75 sur un parc de 3 478 users (cap v2 : 150) restaient
// prisonniers à vie : le rapport était tronqué en PERMANENCE, la
// réconciliation ne tournait plus jamais. v5 découpe le parc en fenêtres :
//   - total=<nb total d'users sur le routeur> : compteur EXACT rapporté par
//     chaque chunk — le cloud connaît le vrai parc dès le 1er ;
//   - start/count : fenêtre [start, start+count) du parc, inlinés par Go
//     depuis le payload de la commande ; out=<entrées émises> ;
//   - sessions : rapportées UNIQUEMENT par le chunk final (start+count >=
//     total) — les chunks intermédiaires n'alourdissent pas leur POST pour
//     rien ; stotal (total de sessions actives) est rapporté par tous ;
//   - trunc=true|false : il RESTE des chunks (start+count < total) —
//     informatif (le cloud décide sur total/start/count, pas sur le drapeau).
//
// v4 (N°75) : board/freehdd/totalhdd en Mo côté script ; ifaces (8 running,
// compteurs cumulés → débits par diff côté cloud). v2 : le gel honnête des
// déductions sur rapport incomplet reste LA règle (chunk perdu = cycle
// abandonné sans déduction, la complétude est vérifiée avant d'appliquer).
func (b Builder) buildReadState(cmd model.Command) string {
	start := int(plInt64(cmd.Payload, "start"))
	count := int(plInt64(cmd.Payload, "count"))
	if start < 0 {
		start = 0
	}
	if count <= 0 || count > ReadChunkSize {
		count = ReadChunkSize
	}
	end := start + count
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(`:local rsres [/system resource get]
:local rver [:tostr ($rsres->"version")]
:local rup [:tostr ($rsres->"uptime")]
:local rcpu [:tostr ($rsres->"cpu-load")]
:local rmem [:tostr ($rsres->"free-memory")]
:local rmemb [:tostr ($rsres->"total-memory")]
:local rboard [:tostr ($rsres->"board-name")]
:local rfreehdd 0
:local rtotalhdd 0
:do {
  :set rfreehdd ([:tonum [:tostr ($rsres->"free-hdd-space")]] / 1048576)
  :set rtotalhdd ([:tonum [:tostr ($rsres->"total-hdd-space")]] / 1048576)
} on-error={ :set rfreehdd 0; :set rtotalhdd 0 }
:local mikIds [/ip hotspot user find]
:local mikTotal [:len $mikIds]
:local rusr ""
:local rout 0
:local n 0
:foreach u in=$mikIds do={
  :if ($n >= @@START@@ && $n < @@END@@) do={
    :set rusr ($rusr . [:tostr [/ip hotspot user get $u name]] . "|" . [:tostr [/ip hotspot user get $u profile]] . "|" . [:tostr [/ip hotspot user get $u disabled]] . ";")
    :set rout ($rout + 1)
  }
  :set n ($n + 1)
}
:local rtrunc "false"
:if (@@END@@ < $mikTotal) do={ :set rtrunc "true" }
:local rstotal [:len [/ip hotspot active find]]
:local rsess ""
:local rsn 0
:if (@@END@@ >= $mikTotal) do={
  :foreach a in=[/ip hotspot active find] do={
    :if ($rsn < 250) do={
      :set rsess ($rsess . [:tostr [/ip hotspot active get $a user]] . "|" . [:tostr [/ip hotspot active get $a address]] . "|" . [:tostr [/ip hotspot active get $a uptime]] . "|" . [:tostr [/ip hotspot active get $a bytes-in]] . "|" . [:tostr [/ip hotspot active get $a bytes-out]] . ";")
      :set rsn ($rsn + 1)
    }
  }
}
:local rsesspart ("&stotal=". $rstotal)
:if (@@END@@ >= $mikTotal) do={
  :set rsesspart ("&stotal=". $rstotal ."&sessions=". $rsess)
}
:local rif ""
:do {
  :local rin 0
  :foreach ifv in=[/interface find] do={
    :if ($rin < 8) do={
      :if ([:tostr [/interface get $ifv running]] = "true") do={
        :set rif ($rif . [:tostr [/interface get $ifv name]] . ":" . [:tostr [/interface get $ifv rx-byte]] . ":" . [:tostr [/interface get $ifv tx-byte]] . ";")
        :set rin ($rin + 1)
      }
    }
  }
} on-error={ :set rif "" }
`)
	sb.WriteString(`/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + cmd.ID +
		`&status=ok&version=". $rver ."&uptime=". $rup ."&cpu=". $rcpu ."&freemem=". $rmem ."&totalmem=". $rmemb` +
		` ."&board=". $rboard ."&freehdd=". $rfreehdd ."&totalhdd=". $rtotalhdd` +
		` ."&total=". $mikTotal ."&start=@@START@@&count=@@COUNT@@&out=". $rout` +
		` ."&users=". $rusr . $rsesspart ."&ifaces=". $rif ."&trunc=". $rtrunc) output=none` + "\n")
	// Placeholders substitués en dernier : une seule chaîne brute lisible,
	// aucune concaténation au milieu du script (le pattern @@VAR@@ ne peut
	// pas apparaître par accident dans une commande RouterOS).
	out := strings.NewReplacer("@@START@@", strconv.Itoa(start), "@@END@@", strconv.Itoa(end), "@@COUNT@@", strconv.Itoa(count)).Replace(sb.String())
	return out
}

// buildReadDhcp — F9 : /ip dhcp-server lease → mac|address|host|expires|status.
func (b Builder) buildReadDhcp(cmd model.Command) string {
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n:local rdata \"\"\n")
	sb.WriteString(":do {\n  :local rn 0\n")
	sb.WriteString("  :foreach le in=[/ip dhcp-server lease find] do={\n")
	sb.WriteString("    :if ($rn < 100) do={\n")
	sb.WriteString(`      :set rdata ($rdata . [:tostr [/ip dhcp-server lease get $le mac-address]] . "|" . [:tostr [/ip dhcp-server lease get $le address]] . "|" . [:tostr [/ip dhcp-server lease get $le host-name]] . "|" . [:tostr [/ip dhcp-server lease get $le expires-after]] . "|" . [:tostr [/ip dhcp-server lease get $le status]] . ";")` + "\n")
	sb.WriteString("      :set rn ($rn + 1)\n")
	sb.WriteString("    }\n  }\n")
	sb.WriteString("} on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.fetchResultData(cmd.ID, okVar))
	return sb.String()
}

// buildReadHosts — F9 : /ip hotspot host → mac|address|server|uptime|authorized
// (authorized=true si l'hôte est bypassed).

// buildReadHosts — F9 : /ip hotspot host → mac|address|server|uptime|authorized
// (authorized=true si l'hôte est bypassed).
func (b Builder) buildReadHosts(cmd model.Command) string {
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n:local rdata \"\"\n")
	sb.WriteString(":do {\n  :local rn 0\n")
	sb.WriteString("  :foreach he in=[/ip hotspot host find] do={\n")
	sb.WriteString("    :if ($rn < 100) do={\n")
	sb.WriteString("      :local hauth \"false\"\n")
	sb.WriteString("      :if ([:tostr [/ip hotspot host get $he bypassed]] = \"true\") do={ :set hauth \"true\" }\n")
	sb.WriteString(`      :set rdata ($rdata . [:tostr [/ip hotspot host get $he mac-address]] . "|" . [:tostr [/ip hotspot host get $he address]] . "|" . [:tostr [/ip hotspot host get $he server]] . "|" . [:tostr [/ip hotspot host get $he uptime]] . "|" . $hauth . ";")` + "\n")
	sb.WriteString("      :set rn ($rn + 1)\n")
	sb.WriteString("    }\n  }\n")
	sb.WriteString("} on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.fetchResultData(cmd.ID, okVar))
	return sb.String()
}

// buildReadCookies — F9 : /ip hotspot cookie → user|mac|expires.

// buildReadCookies — F9 : /ip hotspot cookie → user|mac|expires.
func (b Builder) buildReadCookies(cmd model.Command) string {
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n:local rdata \"\"\n")
	sb.WriteString(":do {\n  :local rn 0\n")
	sb.WriteString("  :foreach ce in=[/ip hotspot cookie find] do={\n")
	sb.WriteString("    :if ($rn < 50) do={\n")
	sb.WriteString(`      :set rdata ($rdata . [:tostr [/ip hotspot cookie get $ce user]] . "|" . [:tostr [/ip hotspot cookie get $ce mac-address]] . "|" . [:tostr [/ip hotspot cookie get $ce expires-in]] . ";")` + "\n")
	sb.WriteString("      :set rn ($rn + 1)\n")
	sb.WriteString("    }\n  }\n")
	sb.WriteString("} on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.fetchResultData(cmd.ID, okVar))
	return sb.String()
}

// buildReadLog — F9 : /log where topics~"hotspot" → time|topics|message,
// 50 DERNIÈRES lignes seulement (le /log find va du plus ancien au plus
// récent : on saute les ltotal-50 premières). Les « | » et « ; » des messages
// sont remplacés par des espaces avant concaténation.

// buildReadLog — F9 : /log where topics~"hotspot" → time|topics|message,
// 50 DERNIÈRES lignes seulement (le /log find va du plus ancien au plus
// récent : on saute les ltotal-50 premières). Les « | » et « ; » des messages
// sont remplacés par des espaces avant concaténation.
func (b Builder) buildReadLog(cmd model.Command) string {
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n:local rdata \"\"\n")
	sb.WriteString(":do {\n")
	sb.WriteString("  :local lall [/log find where topics~\"hotspot\"]\n")
	sb.WriteString("  :local ltotal [:len $lall]\n")
	sb.WriteString("  :local lstart 0\n")
	sb.WriteString("  :if ($ltotal > 50) do={ :set lstart ($ltotal - 50) }\n")
	sb.WriteString("  :local li 0\n")
	sb.WriteString("  :foreach le in=$lall do={\n")
	sb.WriteString("    :if ($li >= $lstart) do={\n")
	sb.WriteString("      :local lmsg [:tostr [/log get $le message]]\n")
	sb.WriteString("      :local lmsgc \"\"\n")
	sb.WriteString("      :if ([:len $lmsg] > 0) do={\n")
	sb.WriteString("        :for lx from=0 to=([:len $lmsg] - 1) do={\n")
	sb.WriteString("          :local lch [:pick $lmsg $lx ($lx + 1)]\n")
	sb.WriteString("          :if ($lch = \"|\" || $lch = \";\") do={ :set lch \" \" }\n")
	sb.WriteString("          :set lmsgc ($lmsgc . $lch)\n")
	sb.WriteString("        }\n")
	sb.WriteString("      }\n")
	sb.WriteString(`      :set rdata ($rdata . [:tostr [/log get $le time]] . "|" . [:tostr [/log get $le topics]] . "|" . $lmsgc . ";")` + "\n")
	sb.WriteString("    }\n")
	sb.WriteString("    :set li ($li + 1)\n")
	sb.WriteString("  }\n")
	sb.WriteString("} on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.fetchResultData(cmd.ID, okVar))
	return sb.String()
}

// buildReadScheduler — F10 : /system scheduler → name|interval|disabled|onevent.
// Le on-event est assaini (« | », « ; », « : » et retours à la ligne → espaces).

// buildReadScheduler — F10 : /system scheduler → name|interval|disabled|onevent.
// Le on-event est assaini (« | », « ; », « : » et retours à la ligne → espaces).
func (b Builder) buildReadScheduler(cmd model.Command) string {
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n:local rdata \"\"\n")
	sb.WriteString(":do {\n  :local rn 0\n")
	sb.WriteString("  :foreach se in=[/system scheduler find] do={\n")
	sb.WriteString("    :if ($rn < 100) do={\n")
	sb.WriteString("      :local sev [:tostr [/system scheduler get $se on-event]]\n")
	sb.WriteString("      :local sevc \"\"\n")
	sb.WriteString("      :if ([:len $sev] > 0) do={\n")
	sb.WriteString("        :for sx from=0 to=([:len $sev] - 1) do={\n")
	sb.WriteString("          :local sch [:pick $sev $sx ($sx + 1)]\n")
	sb.WriteString("          :if ($sch = \"|\" || $sch = \";\" || $sch = \":\") do={ :set sch \" \" }\n")
	sb.WriteString("          :set sevc ($sevc . $sch)\n")
	sb.WriteString("        }\n")
	sb.WriteString("      }\n")
	sb.WriteString(`      :set rdata ($rdata . [:tostr [/system scheduler get $se name]] . "|" . [:tostr [/system scheduler get $se interval]] . "|" . [:tostr [/system scheduler get $se disabled]] . "|" . $sevc . ";")` + "\n")
	sb.WriteString("      :set rn ($rn + 1)\n")
	sb.WriteString("    }\n  }\n")
	sb.WriteString("} on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.fetchResultData(cmd.ID, okVar))
	return sb.String()
}

// buildReadResources — Parité Mikhmon : noms des ressources RouterOS utiles
// aux formulaires — pools d'adresses (/ip pool), files parent (/queue simple,
// hors files dynamiques) et serveurs hotspot (/ip hotspot). Chaque entrée est
// rapportée « kind|name; » (pool|queue|server), relu par parseResourcesRows.

// buildReadResources — Parité Mikhmon : noms des ressources RouterOS utiles
// aux formulaires — pools d'adresses (/ip pool), files parent (/queue simple,
// hors files dynamiques) et serveurs hotspot (/ip hotspot). Chaque entrée est
// rapportée « kind|name; » (pool|queue|server), relu par parseResourcesRows.
func (b Builder) buildReadResources(cmd model.Command) string {
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n:local rdata \"\"\n")
	sb.WriteString(":do {\n  :local rn 0\n")
	sb.WriteString("  :foreach pe in=[/ip pool find] do={\n")
	sb.WriteString("    :if ($rn < 60) do={\n")
	sb.WriteString(`      :set rdata ($rdata . "pool|" . [:tostr [/ip pool get $pe name]] . ";")` + "\n")
	sb.WriteString("      :set rn ($rn + 1)\n")
	sb.WriteString("    }\n  }\n")
	sb.WriteString("  :local qn 0\n")
	sb.WriteString("  :foreach qe in=[/queue simple find] do={\n")
	sb.WriteString("    :if ($qn < 60) do={\n")
	sb.WriteString("      :if ([:tostr [/queue simple get $qe dynamic]] = \"false\") do={\n")
	sb.WriteString(`        :set rdata ($rdata . "queue|" . [:tostr [/queue simple get $qe name]] . ";")` + "\n")
	sb.WriteString("      }\n")
	sb.WriteString("      :set qn ($qn + 1)\n")
	sb.WriteString("    }\n  }\n")
	sb.WriteString("  :local hn 0\n")
	sb.WriteString("  :foreach he in=[/ip hotspot find] do={\n")
	sb.WriteString("    :if ($hn < 20) do={\n")
	sb.WriteString(`      :set rdata ($rdata . "server|" . [:tostr [/ip hotspot get $he name]] . ";")` + "\n")
	sb.WriteString("      :set hn ($hn + 1)\n")
	sb.WriteString("    }\n  }\n")
	sb.WriteString("} on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.fetchResultData(cmd.ID, okVar))
	return sb.String()
}

// intervalPattern — durée RouterOS simple (« 45s », « 5m », « 1h », « 2d », « 1w »).
