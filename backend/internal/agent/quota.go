// N°106 — mode bridage (quota data) : scripts RouterOS du soft-landing.
//
// Besoin produit : un forfait « 1 h / 1 Go » dont le quota data ÉPUISÉ ne
// COUPE PAS la connexion — le débit est BRIDÉ jusqu'à l'expiration du temps.
// RouterOS n'a AUCUN réglage natif pour ça : limit-bytes-total déconnecte
// l'utilisateur dès la limite atteinte (doc MikroTik), et le menu
// /ip hotspot active est INFORMATIONNEL (aucun rate-li-mit modifiable à
// chaud). La construction éprouvée (pattern des forums MikroTik 2010+) :
// une file simple STATIQUE « mikthrottle-<user> » ciblant l'IP du client,
// posée AU-DESSUS de la file dynamique du profil (place-before — les files
// simples sont évaluées en ordre, premier-match gagnant), retirée au départ.
//
// Le quota et le débit de bridage ne vivent PAS dans le profil : ils sont
// embarqués en TÊTE du commentaire routeur de chaque utilisateur/voucher
// sous le marqueur « mikq:<octets>,<débit> » (ex. « mikq:1073741824,512k/512k »).
// Les scripts ci-dessous sont donc GÉNÉRIQUES : sans marqueur, ils ne font
// rien (coût nul pour les profils en mode « cut » — comportement historique
// inchangé). La virgule et le « / » du marqueur ne figurent pas parmi les
// séparateurs neutralisés par l'import (| ; & = % +) ; posé en tête, le
// marqueur survit à la troncature d'import à 60 caractères.
//
// N°113 — LA LEÇON DU TERRAIN (test N°106 chez cybere-space sc : 200 Mo
// consommés sans AUCUN bridage) : la file dynamique du hotspot se nomme
// « <user> » AVEC CHEVRONS (convention RouterOS — cf. sorties réelles du
// forum MikroTik : name="<hotspot-user3>"), et les simple queues suivent
// un ORDRE STRICT premier-match-gagnant (manuel officiel : « each packet
// must go through every queue until it reaches one queue whose conditions
// fit »). L'ancre historique `find where name=$qu` ne trouvait JAMAIS la
// dynamique → la file mikthrottle-<user> était ajoutée en BAS de liste,
// SOUS la dynamique → le bridage ne voyait pas un seul paquet. La pose v2
// place donc la file AVANT LA PREMIÈRE de la liste (place-before, repli
// move-to-top) : au-dessus de la dynamique <user>, du plafond QoS
// mikcloud-qos (N°104) et de toute file créée par l'opérateur — seules les
// files aux cibles DISJOINTES (autres utilisateurs) restent au-dessus,
// sans effet sur le trafic de cet utilisateur.
//
// Les trois temps du bridage :
//  1. ON-LOGIN (profil)   : cumul déjà ≥ quota (re-connexion) → file posée
//     AVANT le premier paquet utile — la fenêtre de
//     re-login est couverte ;
//  2. TICK (scheduler 20 s) : dépassement en cours de session → file posée
//     (place-before la dynamique) ; reset-counters →
//     file retirée ; orphelins balayés ;
//  3. ON-LOGOUT (profil)  : dernière session partie → file retirée (le
//     balayage du tick reste la garantie : l'ordre
//     d'exécution on-logout vs retrait de la session
//     active n'est pas documenté par MikroTik).
//
// La décision se fait sur les compteurs CUMULÉS de l'UTILISATEUR
// (/ip hotspot user bytes-in + bytes-out — survivent aux reconnexions et
// au reboot), jamais sur ceux de la session : un cumul session repart à
// zéro à chaque login et rouvrirait le plein débit (LA faille classique
// de ce montage). limit-uptime (quota temps) reste inchangé : le routeur
// coupe la session à l'heure, le on-logout retire la file, la limite de
// temps refuse la reconnexion — l'utilisateur n'est jamais déconnecté
// pour le QUOTA, seulement pour le TEMPS, exactement le contrat demandé.
//
// Limites documentées (v1) :
//   - shared-users > 1 : une seule file par utilisateur (la dernière IP) ;
//     recommander shared-users=1 sur les profils à quota (le bridage est
//     global au cumul, mais seule l'IP de la session traitée est bridée) ;
//   - le tick traite au plus 250 sessions actives par passage (miroir de la
//     limite sessions du read_state — un parc au-delà converge par rotation) ;
//   - changement d'IP en cours de session sans re-login : la cible de la
//     file n'est pas rafraîchie par le tick (cas quasi inexistant en
//     hotspot — bail DHCP stable ; le on-login re-pose la file à chaque
//     connexion).
package agent

import (
	"fmt"
	"strconv"
	"strings"

	"mikcloud/hotspot-api/internal/model"
)

// QuotaSchedName — nom du scheduler de bridage créé sur le routeur (tick
// 20 s). Miroir du veilleur d'invités mikcloud-watch (N°77).
const QuotaSchedName = "mikcloud-quota"

// QuotaSchedIntervalSec — pas du scheduler de bridage : 20 s. Compromis
// documenté : à cette cadence, la fenêtre de sur-consommation entre
// l'épuisement du quota et la pose de la file vaut ~intervalle × débit du
// profil (ex. 20 s × 10 Mbps ≈ 25 Mo sur un forfait 1 Go = 2,5 %), pour
// un coût CPU routeur négligeable (deux lectures API par tick).
const QuotaSchedIntervalSec = 20

// QuotaTickVersion — GÉNÉRATION du script du tick (N°113). Le tick est figé
// dans le on-event du scheduler ROUTEUR : QuotaSchedOK seul ne suffit pas à
// servir une évolution du script aux routeurs déjà convergés. La version
// voyage dans le payload de quota_ensure et revient dans son rapport — le
// check-in re-file tant que la génération confirmée du routeur n'est pas la
// courante (pattern sel safeWifiRulesVersion N°80).
//
//	v1 (N°106) : ancre `find name=$qu` — ne matchait jamais la dynamique
//	             <user> : file posée en bas de liste, sous la dynamique →
//	             PREMIER-MATCH GAGNANT : AUCUN bridage (terrain cybere-
//	             space sc : 200 Mo sans bridage).
//	v2 (N°113) : place-before la PREMIÈRE file de la liste (repli
//	             move-to-top si le place-before échoue), remove-then-add à
//	             chaque évaluation (rafraîchit la cible IP et la position).
const QuotaTickVersion = 2

// QuotaMarkerPrefix — préfixe du marqueur de quota dans le commentaire
// routeur. « mikq: » fait 5 caractères : les scripts le repèrent par
// [:pick $uc 0 5].
const QuotaMarkerPrefix = "mikq:"

// QuotaThrottlePrefix — préfixe des files de bridage dans /queue simple.
// « mikthrottle- » fait 12 caractères : le tick et le read_state extraient
// le username par [:pick $qn 12 [:len $qn]].
const QuotaThrottlePrefix = "mikthrottle-"

// QuotaMarker — marqueur embarqué en tête du commentaire routeur :
// « mikq:<octets>,<débit> » (ex. « mikq:1073741824,512k/512k »).
func QuotaMarker(bytes int64, rate string) string {
	return fmt.Sprintf("%s%d,%s", QuotaMarkerPrefix, bytes, rate)
}

// PrefixQuotaComment — préfixe le marqueur au commentaire existant. Le
// marqueur est TOUJOURS en tête (il survit à la troncature d'import) et
// se termine à la première espace — le verrou « 1er appareil » peut
// append « mikcloud_lock:MAC » derrière sans le casser.
func PrefixQuotaComment(marker, comment string) string {
	if comment == "" {
		return marker
	}
	return marker + " " + comment
}

// quotaApplyLines — le cœur partagé du on-login et du tick : lit le marqueur
// du commentaire utilisateur, compare au cumul, pose ou retire la file.
// Paramètres RouterOS attendus : $qu (username), $qa (adresse IP de la
// session), $que (id interne /ip hotspot user).
//
// Pose v2 (N°113) — TOUJOURS remove-then-add : la cible IP est rafraîchie à
// chaque évaluation (couvre le changement d'IP sans re-login, limite v1
// documentée) et la position est ré-ancrée avant la PREMIÈRE file de la
// liste — les simple queues s'évaluent en ordre STRICT premier-match
// gagnant : au-dessus de la dynamique <user> et du plafond mikcloud-qos,
// le bridage s'applique réellement. La fenêtre remove→add est sub-
// milliseconde (exécutée d'un bloc par l'interpréteur) ; les compteurs de
// LA FILE de bridage repartent à zéro à chaque tick — purement cosmétique
// (la décision se fait sur les compteurs CUMULÉS de l'UTILISATEUR, jamais
// sur ceux de la file ; le read_state ne rapporte que le NOM des files
// mikthrottle-). Repli en profondeur : si le place-before échoue, la file
// est ajoutée puis REMONTÉE en tête (/queue simple move — l'opération
// historiquement supportée pour placer une statique AVANT les dynamiques,
// pattern des forums 2007+) ; si tout échoue, log sans interrompre.
// Retour une ligne compacte (les séparateurs « ; » sont valides en import
// .rsc, même sérialisation que watcherOnEvent).
const quotaApplyLines = `:local uc [:tostr [/ip hotspot user get $que comment]]; :if ([:pick $uc 0 5] = "mikq:") do={ :local sp [:find $uc " "]; :local mk ""; :if ([:typeof $sp] = "nil") do={ :set mk [:pick $uc 5 [:len $uc]] } else={ :set mk [:pick $uc 5 $sp] }; :local cp [:find $mk ","]; :if ([:typeof $cp] != "nil") do={ :local qb [:pick $mk 0 $cp]; :local qr [:pick $mk ($cp + 1) [:len $mk]]; :local bi 0; :local bo 0; :do { :set bi [:tonum [:tostr [/ip hotspot user get $que bytes-in]]] } on-error={ :set bi 0 }; :do { :set bo [:tonum [:tostr [/ip hotspot user get $que bytes-out]]] } on-error={ :set bo 0 }; :if (($bi + $bo) >= [:tonum $qb]) do={ :local qn ("mikthrottle-" . $qu); :do { /queue simple remove [find where name=$qn] } on-error={}; :local qf [:pick [/queue simple find] 0]; :if ([:len $qf] > 0) do={ :do { /queue simple add name=$qn target=$qa max-limit=$qr place-before=$qf } on-error={ :do { /queue simple add name=$qn target=$qa max-limit=$qr; :local tq [/queue simple find where name=$qn]; :local qf2 [:pick [/queue simple find] 0]; :if ([:len $tq] > 0 && [:len $qf2] > 0 && ([:pick $tq 0] != $qf2)) do={ /queue simple move [:pick $tq 0] $qf2 } } on-error={ :log info "mikcloud: quota bridage impossible" } } } else={ :do { /queue simple add name=$qn target=$qa max-limit=$qr } on-error={ :log info "mikcloud: quota bridage impossible" } } } else={ :do { /queue simple remove [find where name=("mikthrottle-" . $qu)] } on-error={} } } }`

// onLoginQuotaScript — script on-login du profil en mode throttle : à
// CHAQUE connexion, si le cumul de l'utilisateur a déjà épuisé le quota du
// marqueur, la file de bridage est (re)posée pour l'IP de CETTE session
// (remove-then-add : la cible est rafraîchie à chaque connexion, la file
// ré-ancrée EN TÊTE de liste — la dynamique <user> créée au même login ne
// peut pas la recouvrir). C'est la couverture de la fenêtre de re-login :
// sans elle, un client dont le quota est épuisé repartirait au débit plein
// entre l'épuisement et le tick. Les variables $user/$address sont fournies
// par le hotspot au login.
const onLoginQuotaScript = `:do { :local qu $user; :local qa $address; :local que [/ip hotspot user find name=$qu]; :if ([:len $que] > 0) do={ ` + quotaApplyLines + ` } } on-error={ :log info "mikcloud: quota on-login ignore" }`

// onLogoutQuotaScript — script on-logout du profil en mode throttle :
// quand la DERNIÈRE session de l'utilisateur se ferme (kick, logout,
// expiration limit-uptime), la file de bridage est retirée — sinon elle
// survivrait à la session et briderait le PROCHAIN occupant de l'IP (bail
// DHCP réattribué). Si d'autres sessions restent actives (shared-users > 1),
// la file reste (le cumul est partagé). Le balayage d'orphelins du tick
// reste la garantie : l'ordre d'exécution du on-logout par rapport au
// retrait de la session dans /ip hotspot active n'est pas documenté.
const onLogoutQuotaScript = `:do { :local qu $user; :if ([:len [/ip hotspot active find where user=$qu]] = 0) do={ :do { /queue simple remove [find where name=("mikthrottle-" . $qu)] } on-error={} } } on-error={ :log info "mikcloud: quota on-logout ignore" }`

// quotaTickScript — corps UNE LIGNE du scheduler mikcloud-quota (tick 20 s).
// Trois passes, chacune protégée (un utilisateur au marqueur corrompu
// n'interrompt jamais les autres) :
//  1. ORPHELINS — toute file mikthrottle-<user> sans session active →
//     retirée (garantie anti-héritage d'IP, y compris après reboot du
//     routeur : les files simples statiques survivent au reboot, les
//     sessions non) ;
//  2. sessions actives (≤ 250 par tick) — marqueur mikq: lu, cumul
//     comparé : ≥ quota → file posée EN TÊTE (place-before la première,
//     cf. quotaApplyLines v2) ; < quota → file retirée si présente
//     (reset-counters F4 rouvre le plein débit dans le tick suivant) ;
//  3. rien d'autre — aucun octet émis vers le cloud : le tick est 100 %
//     local, le routeur continue d'appliquer la politique même coupé du
//     cloud (philosophie agent MikCloud).
const quotaTickScript = `:do { :foreach q in=[/queue simple find where name~"^mikthrottle-"] do={ :do { :local qn [:tostr [/queue simple get $q name]]; :local unm [:pick $qn 12 [:len $qn]]; :if ([:len [/ip hotspot active find where user=$unm]] = 0) do={ /queue simple remove $q } } on-error={} }; :local qn2 0; :foreach a in=[/ip hotspot active find] do={ :if ($qn2 < 250) do={ :set qn2 ($qn2 + 1); :do { :local qu [:tostr [/ip hotspot active get $a user]]; :local qa [:tostr [/ip hotspot active get $a address]]; :local que [/ip hotspot user find name=$qu]; :if ([:len $que] > 0) do={ ` + quotaApplyLines + ` } } on-error={} } } } on-error={ :log info "mikcloud: quota tick ignore" }`

// buildQuotaEnsure — N°106 : déploie (ou redéploie) le scheduler de bridage
// sur un routeur agent : remove-then-add idempotent, rapporté comme toute
// commande (pattern watcher N°77). Servi aux routeurs agents dont
// QuotaSchedOK est faux ET dont le compte possède au moins un profil en
// mode throttle (ensureQuotaThrottleLocked au check-in — convergence
// automatique du parc en UN check-in, re-file tant que le retour « ok »
// n'est pas arrivé). Le tick est inerte sans marqueur mikq: : un routeur
// dont le compte repasse tous ses profils en « cut » ne consomme rien, le
// scheduler n'est simplement jamais retiré (documenté — sa suppression
// n'est pas garantie par le protocole agent en v1).
func (b Builder) buildQuotaEnsure(cmd model.Command) string {
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do {\n  /system scheduler remove [find name=\"" + QuotaSchedName + "\"]\n} on-error={}\n")
	sb.WriteString(":do {\n  /system scheduler add name=\"" + QuotaSchedName + "\" interval=" + strconv.Itoa(QuotaSchedIntervalSec) + "s start-time=startup on-event=\"" +
		rosEscape(quotaTickScript) + "\"\n} on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

// profileOnLoginScript — on-login COMBINÉ du profil : le verrou « 1er
// appareil » (liaison MAC) ET le bridage quota peuvent être actifs
// simultanément — RouterOS n'offre qu'UN champ on-login par profil, les
// deux blocs :do{…}on-error={} indépendants sont concaténés (chacun
// ignore l'échec de l'autre).
func profileOnLoginScript(p ProfileRef) string {
	script := ""
	if p.LockFirstDevice {
		script += onLoginLockScript
	}
	if p.QuotaThrottle {
		script += onLoginQuotaScript
	}
	return script
}
