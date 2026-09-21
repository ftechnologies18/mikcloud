// Opérations utilisateurs/vouchers : création, lot, modification, kick, reset.
// Extrait du monolithe agent.go (N°88) — même package, contenu inchangé.
package agent

import (
	"fmt"
	"mikcloud/hotspot-api/internal/model"
	"strings"
)

// onLoginLockScript — script on-login du verrou « 1er appareil » (v2,
// anti-partage). À la première connexion d'un utilisateur du profil, l'adresse
// MAC de l'appareil ($"caller-id") est mémorisée dans le commentaire routeur
// sous la marque « mikcloud_lock: » (append : le commentaire de traçabilité
// MikCloud, ex. lot de vouchers, est préservé). Aux connexions suivantes, tout
// autre appareil est déconnecté immédiatement. Le commentaire routeur n'est
// JAMAIS relu par le cloud (import = name|profile|disabled) : la liaison MAC
// reste un état local au routeur. Une seule ligne : les valeurs de propriété
// RouterOS sont embarquées entre guillemets (voir rosScriptValue).
const onLoginLockScript = `:do {:local m $"caller-id";:local u $user;:if ([:len $m] > 0) do={:local e [/ip hotspot user find name=$u];:if ([:len $e] > 0) do={:local c [:tostr [/ip hotspot user get $e comment]];:local i [:find $c "mikcloud_lock:"];:if ([:typeof $i] = "nil") do={:if ([:len $c] = 0) do={/ip hotspot user set $e comment=("mikcloud_lock:" . $m)} else={/ip hotspot user set $e comment=($c . " mikcloud_lock:" . $m)}} else={:local lm [:pick $c ($i + 14) [:len $c]];:if ($lm != $m) do={/ip hotspot active remove [find user=$u]}}}}} on-error={ :log info "mikcloud: liaison mac ignoree" }`

// rosScriptValue — échappe un SCRIPT RouterOS pour l'embarquer dans une valeur
// de propriété entre guillemets (ex. on-login) : le script interne doit
// survivre au parsing de la ligne externe ($, " et \ protégés).

// rosScriptValue — échappe un SCRIPT RouterOS pour l'embarquer dans une valeur
// de propriété entre guillemets (ex. on-login) : le script interne doit
// survivre au parsing de la ligne externe ($, " et \ protégés).
func rosScriptValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, `$`, `\$`)
	return s
}

// profileEnsureLine — garantit que le profil du cloud existe sur le routeur
// avec ses paramètres EXACTS : add (création) puis set (alignement si le
// profil existait déjà — créé dans Winbox ou par un import). Sans ce set, un
// profil préexistant sans session-timeout restait sans timeout → les vouchers
// n'expiraient jamais. Le set est inconditionnel : sur un profil absent,
// `set [find …]` sans résultat est un no-op silencieux en RouterOS, jamais
// une erreur. Deux lignes indépendantes plutôt qu'un :do imbriqué : une
// erreur de l'add n'empêche jamais le set. Renvoie TROIS lignes terminées (add, set, quota rétroactif des utilisateurs existants).

func (b Builder) buildUserAdd(cmd model.Command) string {
	name := SanitizeName(plStr(cmd.Payload, "name"))
	pass := plStr(cmd.Payload, "password")
	prof := plProfile(cmd.Payload, "profile")
	comment := plStr(cmd.Payload, "comment")
	quota := plInt64(cmd.Payload, "limitBytesTotal")
	// Parité Mikhmon : limit-uptime du voucher (minutes) — le payload prime,
	// sinon héritage du session-timeout du profil (0 = illimité).
	uptime := plInt64(cmd.Payload, "limitUptimeMin")
	if uptime <= 0 && prof.HasTimeout && prof.SessionTimeoutMin > 0 {
		uptime = int64(prof.SessionTimeoutMin)
	}
	server := plStr(cmd.Payload, "server")
	// N°106 — débit de bridage du profil (mode throttle uniquement).
	throttleRate := plStr(cmd.Payload, "throttleRate")
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(profileEnsureLine(prof))
	line := `/ip hotspot user add name="` + rosEscape(name) + `"`
	if pass != "" {
		line += ` password="` + rosEscape(pass) + `"`
	}
	line += ` profile="` + rosEscape(prof.Name) + `"`
	// Quota de temps TOTAL du ticket (parité Mikhmon) : limit-uptime du
	// voucher (override par lot) ou, à défaut, la durée du profil. Une fois
	// le cumul épuisé, le routeur refuse la reconnexion (« no more time »).
	if uptime > 0 {
		line += " limit-uptime=" + rosMinutes(int(uptime))
	}
	// Parité Mikhmon : serveur hotspot RouterOS visé ("all" ou nom précis).
	if server != "" {
		line += ` server="` + rosEscape(server) + `"`
	}
	// N°106 — mode bridage : en mode throttle, le quota data ne devient PAS un
	// limit-bytes-total (le routeur DÉCONNECTERAIT à l'épuisement — l'inverse du
	// contrat) : il est embarqué en TÊTE du commentaire sous le marqueur
	// mikq:<octets>,<débit>, relu par les scripts on-login/on-logout du profil
	// et par le tick mikcloud-quota. La virgule et le « / » du marqueur ne
	// figurent pas parmi les séparateurs neutralisés par l'import (| ; & = % +).
	if prof.QuotaThrottle && quota > 0 && throttleRate != "" {
		comment = PrefixQuotaComment(QuotaMarker(quota, throttleRate), comment)
	} else if quota > 0 {
		// Quota de données : limit-bytes-total (in + out cumulés, in/out laissés à 0).
		line += fmt.Sprintf(" limit-bytes-total=%d", quota)
	}
	if comment != "" {
		line += ` comment="` + rosEscape(comment) + `"`
	}
	sb.WriteString(":do { " + line + " } on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

func (b Builder) buildVoucherBatch(cmd model.Command) string {
	prof := plProfile(cmd.Payload, "profile")
	users := plUserList(cmd.Payload, "users")
	batch := plStr(cmd.Payload, "batch")
	custom := plStr(cmd.Payload, "comment")
	quota := plInt64(cmd.Payload, "limitBytesTotal")
	// Parité Mikhmon : limit-uptime par lot (minutes) — le payload prime,
	// sinon héritage du session-timeout du profil (0 = illimité).
	uptime := plInt64(cmd.Payload, "limitUptimeMin")
	if uptime <= 0 && prof.HasTimeout && prof.SessionTimeoutMin > 0 {
		uptime = int64(prof.SessionTimeoutMin)
	}
	server := plStr(cmd.Payload, "server")
	// N°106 — débit de bridage du profil (mode throttle uniquement).
	throttleRate := plStr(cmd.Payload, "throttleRate")
	// N°162 — réparation : commande d'autoréparation émise par la
	// réconciliation read_state pour les absents du routeur. Idempotente
	// (un nom déjà présent n'est ni recompté ni retouché) et véridique
	// (même compteur d'échecs que la génération).
	repair := plBool(cmd.Payload, "repair")
	okVar := "ok" + idSafe(cmd.ID)
	failsVar := "fails" + idSafe(cmd.ID)
	// Commentaire router : la traçabilité MikCloud (lot) reste toujours présente ;
	// le commentaire libre du gérant est préfixé devant s'il existe.
	comment := ""
	switch {
	case custom != "" && batch != "":
		comment = custom + " · mikcloud:" + batch
	case custom != "":
		comment = custom
	case batch != "":
		comment = "mikcloud:" + batch
	}
	// N°106 — le mode bridage est une propriété du PROFIL ; le marqueur
	// mikq: est posé PAR VOUCHER ci-dessous (N°162 : la réparation porte
	// les quotas résolus à la génération, individuels par ticket).
	throttled := prof.QuotaThrottle && throttleRate != ""
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":local " + failsVar + " 0\n")
	sb.WriteString(profileEnsureLine(prof))
	for _, u := range users {
		// N°162 — limites PAR VOUCHER : la réparation recompose les valeurs
		// résolues à la génération (stockées par ticket) ; la génération
		// classique n'en pose pas et hérite du lot (comportement inchangé).
		uQuota := u.LimitBytesTotal
		if uQuota == 0 {
			uQuota = quota
		}
		uUptime := u.LimitUptimeMin
		if uUptime == 0 {
			uUptime = uptime
		}
		uComment := comment
		if throttled && uQuota > 0 {
			// N°106 — marqueur mikq:<octets>,<débit> en TÊTE (survit à la
			// troncature d'import à 60 caractères) : le quota ne devient PAS
			// un limit-bytes-total (le routeur déconnecterait à l'épuisement).
			uComment = PrefixQuotaComment(QuotaMarker(uQuota, throttleRate), comment)
		}
		name := SanitizeName(u.Name)
		line := `/ip hotspot user add name="` + rosEscape(name) + `" password="` + rosEscape(u.Password) +
			`" profile="` + rosEscape(prof.Name) + `"`
		// Quota de temps TOTAL du ticket (parité Mikhmon, cf. buildUserAdd) :
		// limit-uptime du lot (override) ou du profil ; cumul épuisé = refus.
		if uUptime > 0 {
			line += " limit-uptime=" + rosMinutes(int(uUptime))
		}
		// Parité Mikhmon : serveur hotspot RouterOS visé ("all" ou nom précis).
		if server != "" {
			line += ` server="` + rosEscape(server) + `"`
		}
		if uQuota > 0 && !throttled {
			// Quota de données du lot (ex. « 5 Go = 500 F ») : limit-bytes-total
			// en octets — le routeur déconnecte le voucher une fois épuisé (mode
			// « cut » ; en mode throttle le quota vit dans le marqueur mikq: du
			// commentaire, posé ci-dessus).
			line += fmt.Sprintf(" limit-bytes-total=%d", uQuota)
		}
		if uComment != "" {
			line += ` comment="` + rosEscape(uComment) + `"`
		}
		// N°162 — VÉRITÉ du lot : un add qui échoue est COMPTÉ. L'ancien
		// script avalait l'échec dans un log routeur et rapportait « ok »
		// même sans AUCUN utilisateur créé (incident Zikisso : lot complet
		// badgé « absent du routeur », tickets invendables — et sans aucune
		// retrouvabilité, les écritures n'étant jamais rejouées).
		if repair {
			// Idempotence de la réparation : un nom DÉJÀ présent compte pour
			// réussi (c'est l'état voulu) sans retoucher l'existant (verrou MAC,
			// marqueur mikq:, comment de traçabilité préservés).
			sb.WriteString(`:if ([:len [/ip hotspot user find name="` + rosEscape(name) + `"]] = 0) do={ :do { ` + line +
				` } on-error={ :set ` + failsVar + ` (` + failsVar + ` + 1); :log warning "mikcloud: add voucher echoue" } }` + "\n")
		} else {
			sb.WriteString(`:do { ` + line + ` } on-error={ :set ` + failsVar + ` (` + failsVar + ` + 1); :log warning "mikcloud: add voucher echoue" }` + "\n")
		}
	}
	// N°162 — le lot passe « error » dès le PREMIER échec : compteurs
	// créés/en échec dans le rapport (pattern dynamique $step du N°159).
	sb.WriteString(":if ($" + failsVar + " > 0) do={ :set " + okVar + " false }\n")
	okLine := b.reportLine(cmd.ID, true, map[string]string{"created": fmt.Sprintf("%d", len(users))})
	koLine := `/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + urlEscape(cmd.ID) +
		`&status=error&message=ajouts_en_echec_sur_le_routeur&created=" . (` +
		fmt.Sprintf("%d", len(users)) + ` - $` + failsVar + `) . "&failed=" . $` + failsVar + `) output=none`
	sb.WriteString(":if ($" + okVar + ") do={\n  " + okLine + "\n} else={\n  " + koLine + "\n}\n")
	return sb.String()
}

func (b Builder) buildUserRemove(cmd model.Command) string {
	names := plStrList(cmd.Payload, "names")
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	for _, n := range names {
		sn := rosEscape(SanitizeName(n))
		sb.WriteString(":do { /ip hotspot user remove [find name=\"" + sn + "\"] } on-error={ :set " + okVar + " false }\n")
		sb.WriteString(":do { /ip hotspot active remove [find user=\"" + sn + "\"] } on-error={ :log info \"mikcloud: session deja fermee\" }\n")
	}
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

func (b Builder) buildUserSet(cmd model.Command) string {
	old := SanitizeName(plStr(cmd.Payload, "oldName"))
	name := SanitizeName(plStr(cmd.Payload, "name"))
	prof := plProfile(cmd.Payload, "profile")
	okVar := "ok" + idSafe(cmd.ID)
	if old == "" {
		old = name
	}
	set := `/ip hotspot user set [find name="` + rosEscape(old) + `"]`
	if name != "" && name != old {
		set += ` name="` + rosEscape(name) + `"`
	}
	if prof.Name != "" {
		set += ` profile="` + rosEscape(prof.Name) + `"`
	}
	if pw := plStr(cmd.Payload, "password"); pw != "" {
		set += ` password="` + rosEscape(pw) + `"`
	}
	if plBool(cmd.Payload, "disabled") {
		set += " disabled=yes"
	} else if plHas(cmd.Payload, "disabled") {
		set += " disabled=no"
	}
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do { " + set + " } on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

func (b Builder) buildKick(cmd model.Command) string {
	user := rosEscape(SanitizeName(plStr(cmd.Payload, "user")))
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":do { /ip hotspot active remove [find user=\"" + user + "\"] } on-error={ :log info \"mikcloud: session introuvable\" }\n")
	sb.WriteString(b.reportLine(cmd.ID, true, nil) + "\n")
	return sb.String()
}

// buildUserReset — F4 : remet à zéro les compteurs (bytes/uptime) d'un
// utilisateur hotspot côté routeur (/ip hotspot user reset-counters).

// buildUserReset — F4 : remet à zéro les compteurs (bytes/uptime) d'un
// utilisateur hotspot côté routeur (/ip hotspot user reset-counters).
func (b Builder) buildUserReset(cmd model.Command) string {
	name := rosEscape(SanitizeName(plStr(cmd.Payload, "name")))
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do { /ip hotspot user reset-counters [find name=\"" + name + "\"] } on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

// ---------------------------------------------------------------------------
// Builders P1 (audit Mikhmon) — F6/F7/F8/F9/F10
// ---------------------------------------------------------------------------
//
// Convention des commandes read_* (F9/F10) : les lignes sont collectées dans la
// variable $rdata au format « champ|champ|champ;champ|… » — champs séparés par
// « | » (les valeurs MAC/IP contiennent « : », un séparateur « : » serait
// ambigu) et entrées par « ; », comme users/sessions de read_state. Le cloud
// relit ce format avec splitAgentList.
//
// Les champs libres (messages de log, on-event) sont assainis CÔTÉ SCRIPT
// (substitution de « | » et « ; » — et « : » pour on-event — par des espaces,
// boucle caractère par caractère : RouterOS n'a pas de replace natif) ; le
// parseur cloud tolère de plus les caractères résiduels (contrat F9).

// fetchResultOK / fetchResultErr — les deux /tool fetch de rapport des
// commandes read_* (ok transporte $rdata, calculé côté routeur).

// VoucherRef — ligne de voucher pour un batch.
type VoucherRef struct {
	Name     string
	Password string
	// N°162 — limites résolues PAR VOUCHER (posées uniquement par la
	// réparation des absents : quotas/uptime stockés par ticket à la
	// génération ; un lot de GÉNÉRATION classique n'envoie que
	// name/password et hérite des valeurs du lot).
	LimitBytesTotal int64
	LimitUptimeMin  int64
}

func plUserList(p map[string]any, k string) []VoucherRef {
	// Le payload peut venir de la mémoire (types concrets Go : []map[string]any)
	// ou d'une relecture JSON ([]any) — accepter les deux formes.
	collect := func(items []any) []VoucherRef {
		out := make([]VoucherRef, 0, len(items))
		for _, it := range items {
			if m, ok := it.(map[string]any); ok {
				out = append(out, VoucherRef{
					Name: plStr(m, "name"), Password: plStr(m, "password"),
					LimitBytesTotal: plInt64(m, "limitBytesTotal"),
					LimitUptimeMin:  plInt64(m, "limitUptimeMin"),
				})
			}
		}
		return out
	}
	if raw, ok := p[k].([]any); ok {
		return collect(raw)
	}
	if raw2, ok := p[k].([]map[string]any); ok {
		items := make([]any, 0, len(raw2))
		for _, it := range raw2 {
			items = append(items, it)
		}
		return collect(items)
	}
	return nil
}
