// N°97 — Docteur du pool d'adresses IP du hotspot.
//
// Constat production (capture gérant 13/09/2026) : aux heures de pointe, le
// portail affiche « cannot assign ip address - no more free addresses from
// pool » — le pool IP du hotspot est ÉPUISÉ. Trois causes conjointes :
//
//  1. pool trop petit (un /24 = 254 adresses partagées entre clients
//     PAYANTS et tous les appareils à portée qui réclament une IP AVANT
//     login — le portail captif exige une IP pour s'afficher) ;
//  2. « zombies » : RouterOS garde l'hôte (et son IP) indéfiniment quand
//     login-timeout n'est pas posé (défaut : aucun) — les téléphones en
//     connexion auto qui ne se connectent jamais squattent le pool ;
//  3. address-per-mac = 2 par défaut : un même appareil peut prendre
//     DEUX adresses.
//
// La commande pool_doctor fait trois choses, chacune idempotente :
//
//   - DIAGNOSTIC (toujours, lecture seule) : rapporte pools (nom + ranges),
//     serveurs hotspot (profil, interface, timeouts), profils (address-pool,
//     address-per-mac), hôtes (IP attribuées, authentifiés ou non) et
//     sessions actives → le cloud calcule la capacité (PoolCap) et suit
//     l'occupation (PoolHosts, alimenté aussi par read_state) pour l'alerte
//     « pool plein » ;
//   - RECYCLAGE (payload recycle, défaut true côté endpoint console) :
//     pose login-timeout=5m, idle-timeout=10m, keepalive-timeout=2m sur les
//     serveurs hotspot et address-per-mac=1 sur les profils — libère les IP
//     des zombies sans toucher au subnet, zéro impact pour les sessions
//     payantes (le cookie hotspot re-connecte l'usager au réveil de son
//     écran ; le solde du voucher est intact) ;
//   - EXTENSION (payload extend, opt-in explicite du gérant) : ajoute le
//     range dédié 10.77.0.10-10.77.7.254 (~2 037 IP) au pool référencé par
//     chaque profil hotspot, pose l'IP secondaire 10.77.0.1/21 sur
//     l'interface hotspot (ARP + passerelle), l'entrée /ip hotspot network
//     masquerade et la règle NAT mikcloud-pool-nat (match src uniquement —
//     aucune interface WAN à deviner). Un profil sans pool reçoit le pool
//     dédié « mikcloud-pool ». Les clients connectés ne sont pas déconnectés :
//     seules les NOUVELLES attributions tirent du range étendu.
//
// Le rapport repart APRÈS application : les ranges rapportés incluent
// l'extension, la capacité vue par le cloud est la vérité routeur.
//
// Sécurité du protocole : les valeurs rapportées ne contiennent jamais
// « | ; & = % » (noms nettoyés caractère par caractère par la fonction
// mikClean DÉFINIE dans le script, pattern buildReadLog) — le POST
// /agent/result (form-encodé) reste parsable.
package agent

import (
	"mikcloud/hotspot-api/internal/model"
	"strings"
	"time"
)

// PoolDoctorExtRange — range d'extension par défaut : 10.77.0.10 → 10.77.7.254
// (plage 10.77.0.0/21 dédiée à MikCloud, ~2 037 adresses). Choix 10.77 :
// hors des usages privés courants (192.168.x, 10.5.x, 10.10.x Mikhmon,
// 172.16-31.x) — un conflit avec un réseau existant est improbable, et le
// docteur reste sûr même en chevauchement (chaque add est sous on-error).
const PoolDoctorExtRange = "10.77.0.10-10.77.7.254"

// PoolDoctorExtSubnet — réseau de l'extension (entrée hotspot network + NAT).
const PoolDoctorExtSubnet = "10.77.0.0/21"

// PoolDoctorExtGateway — IP secondaire posée sur l'interface hotspot
// (ARP + passerelle des clients du range étendu).
const PoolDoctorExtGateway = "10.77.0.1/21"

// PoolDoctorPoolName — nom du pool dédié créé pour un profil sans pool.
const PoolDoctorPoolName = "mikcloud-pool"

// PoolDoctorRefresh — l'auto-diagnostic (recyclage/extension OFF, lecture
// seule) du check-in est re-filé au-delà de cet âge du dernier diagnostic
// (PoolDoctorAt) — les ranges changent rarement, une semaine suffit.
const PoolDoctorRefresh = 7 * 24 * time.Hour

// sanitizeRosToken — borne une valeur pilotée par payload aux caractères
// sûrs pour une commande RouterOS (anti-injection dans le .rsc importé).
func sanitizeRosToken(s string) string {
	var sb strings.Builder
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z',
			c == '.', c == '-', c == '/', c == ':':
			sb.WriteRune(c)
		default:
			return "" // caractère hostile : la valeur est rejetée en bloc
		}
	}
	return sb.String()
}

// buildPoolDoctor — génère le script .rsc (cf. en-tête de fichier).
func (b Builder) buildPoolDoctor(cmd model.Command) string {
	recycle := plBool(cmd.Payload, "recycle")
	extend := plBool(cmd.Payload, "extend")
	extRange := sanitizeRosToken(plStr(cmd.Payload, "extRange"))
	extSubnet := sanitizeRosToken(plStr(cmd.Payload, "extSubnet"))
	extGateway := sanitizeRosToken(plStr(cmd.Payload, "extGateway"))
	loginTO := sanitizeRosToken(plStr(cmd.Payload, "loginTimeout"))
	idleTO := sanitizeRosToken(plStr(cmd.Payload, "idleTimeout"))
	keepaliveTO := sanitizeRosToken(plStr(cmd.Payload, "keepaliveTimeout"))
	// Garde-fous : valeurs par défaut sur payload vide (l'auto-diagnostic du
	// check-in ne porte PAS ces clés). Un token rejeté par sanitizeRosToken
	// retombe aussi sur le défaut : jamais de valeur partielle injectée.
	if extRange == "" {
		extRange = PoolDoctorExtRange
	}
	if extSubnet == "" {
		extSubnet = PoolDoctorExtSubnet
	}
	if extGateway == "" {
		extGateway = PoolDoctorExtGateway
	}
	if loginTO == "" {
		loginTO = "5m"
	}
	if idleTO == "" {
		idleTO = "10m"
	}
	if keepaliveTO == "" {
		keepaliveTO = "2m"
	}

	var sb strings.Builder
	sb.WriteString(header(cmd))
	// mikClean — fonction LOCALE du script : remplace les séparateurs du
	// protocole de rapport (« | ; & = % ») par « _ » dans les valeurs
	// dynamiques (pattern du nettoyage caractère par caractère de
	// buildReadLog, factorisé ici car le docteur rapporte 4 listes).
	sb.WriteString(`:local mikClean do={
  :local mikS [:tostr $1]
  :local mikOut ""
  :if ([:len $mikS] > 0) do={
    :for mikI from=0 to=([:len $mikS] - 1) do={
      :local mikC [:pick $mikS $mikI ($mikI + 1)]
      :if ($mikC = "|" || $mikC = ";" || $mikC = "&" || $mikC = "=" || $mikC = "%") do={ :set mikC "_" }
      :set mikOut ($mikOut . $mikC)
    }
  }
  :return $mikOut
}
:local mkHosts 0
:do { :set mkHosts [/ip hotspot host print count-only] } on-error={}
:local mkActive 0
:do { :set mkActive [/ip hotspot active print count-only] } on-error={}
:local mkRecycled "no"
:local mkExtended "no"
`)
	// ── Recyclage : timeouts serveurs + address-per-mac profils ──
	if recycle {
		sb.WriteString(`:do {
  /ip hotspot set [find] login-timeout=` + loginTO + ` idle-timeout=` + idleTO + ` keepalive-timeout=` + keepaliveTO + `
  :set mkRecycled "yes"
} on-error={ :log warning "mikcloud: recyclage timeouts hotspot impossible" }
:do {
  /ip hotspot profile set [find] address-per-mac=1
  :set mkRecycled "yes"
} on-error={ :log warning "mikcloud: address-per-mac inaccessible (RouterOS < 6.44 ?)" }
`)
	}
	// ── Extension : range dédié + IP secondaire + network + NAT ──
	if extend {
		sb.WriteString(`:do {
  :foreach pe in=[/ip hotspot profile find] do={
    :local pap ""
    :do { :set pap [:tostr [/ip hotspot profile get $pe address-pool]] } on-error={}
    :if ([:len $pap] > 0 && $pap != "none" && $pap != "(unknown)") do={
      :local pid [/ip pool find name=$pap]
      :if ([:len $pid] > 0) do={
        :local pr [:tostr [/ip pool get $pid ranges]]
        :if ([:typeof [:find $pr "` + extRange + `"]] = "nil") do={
          :do {
            /ip pool set $pid ranges=($pr . "," . "` + extRange + `")
            :set mkExtended "yes"
          } on-error={ :log warning "mikcloud: extension du pool impossible" }
        } else={
          :set mkExtended "yes"
        }
      }
    } else={
      :if ([:len [/ip pool find name="` + PoolDoctorPoolName + `"]] = 0) do={
        :do { /ip pool add name="` + PoolDoctorPoolName + `" ranges="` + extRange + `" } on-error={}
      }
      :do {
        /ip hotspot profile set $pe address-pool="` + PoolDoctorPoolName + `"
        :set mkExtended "yes"
      } on-error={ :log warning "mikcloud: pool dedie non posable sur le profil" }
    }
  }
} on-error={}
:do {
  :foreach se in=[/ip hotspot find] do={
    :local sif ""
    :do { :set sif [:tostr [/ip hotspot get $se interface]] } on-error={}
    :if ([:len $sif] > 0) do={
      :if ([:len [/ip address find where address="` + extGateway + `" and interface=$sif]] = 0) do={
        :do { /ip address add address="` + extGateway + `" interface=$sif comment="mikcloud-pool" } on-error={ :log warning "mikcloud: IP secondaire pool impossible (chevauchement ?)" }
      }
    }
  }
} on-error={}
:do {
  :if ([:len [/ip hotspot network find where address="` + extSubnet + `"]] = 0) do={
    /ip hotspot network add address="` + extSubnet + `" masquerade=yes comment="mikcloud-pool"
  }
} on-error={ :log warning "mikcloud: entree hotspot network impossible" }
:do {
  :if ([:len [/ip firewall nat find where comment="mikcloud-pool-nat"]] = 0) do={
    /ip firewall nat add chain=srcnat src-address="` + extSubnet + `" action=masquerade comment="mikcloud-pool-nat"
  }
} on-error={ :log warning "mikcloud: regle NAT pool impossible" }
`)
	}
	// ── Diagnostic : pools (ranges APRÈS extension), serveurs, profils ──
	sb.WriteString(`:local mkPools ""
:do {
  :foreach pe in=[/ip pool find] do={
    :if ([:len $mkPools] < 3000) do={
      :local pn [:tostr [/ip pool get $pe name]]
      :local pr ""
      :do { :set pr [:tostr [/ip pool get $pe ranges]] } on-error={}
      :set mkPools ($mkPools . [$mikClean $pn] . "|" . [$mikClean $pr] . ";")
    }
  }
} on-error={}
:local mkSrv ""
:do {
  :foreach se in=[/ip hotspot find] do={
    :local sn [:tostr [/ip hotspot get $se name]]
    :local sp ""
    :do { :set sp [:tostr [/ip hotspot get $se profile]] } on-error={}
    :local si ""
    :do { :set si [:tostr [/ip hotspot get $se interface]] } on-error={}
    :local slt ""
    :do { :set slt [:tostr [/ip hotspot get $se login-timeout]] } on-error={}
    :local sid ""
    :do { :set sid [:tostr [/ip hotspot get $se idle-timeout]] } on-error={}
    :local skt ""
    :do { :set skt [:tostr [/ip hotspot get $se keepalive-timeout]] } on-error={}
    :set mkSrv ($mkSrv . [$mikClean $sn] . "|" . [$mikClean $sp] . "|" . [$mikClean $si] . "|" . [$mikClean $slt] . "|" . [$mikClean $sid] . "|" . [$mikClean $skt] . ";")
  }
} on-error={}
:local mkProf ""
:do {
  :foreach pe in=[/ip hotspot profile find] do={
    :local pn [:tostr [/ip hotspot profile get $pe name]]
    :local pap ""
    :do { :set pap [:tostr [/ip hotspot profile get $pe address-pool]] } on-error={}
    :local pam ""
    :do { :set pam [:tostr [/ip hotspot profile get $pe address-per-mac]] } on-error={}
    :set mkProf ($mkProf . [$mikClean $pn] . "|" . [$mikClean $pap] . "|" . [$mikClean $pam] . ";")
  }
} on-error={}
:local mkData ("cmd=` + cmd.ID + `&status=ok&recycled=". $mkRecycled ."&extended=". $mkExtended ."&hosts=". $mkHosts ."&active=". $mkActive ."&pools=". $mkPools ."&servers=". $mkSrv ."&profiles=". $mkProf)
`)
	sb.WriteString(`/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=($mkData) output=none` + "\n")
	return sb.String()
}
