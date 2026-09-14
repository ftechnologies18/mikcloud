// N°97 → N°108 — Docteur du pool d'adresses IP du hotspot.
//
// Constat production (capture gérant 13/09/2026) : aux heures de pointe, le
// portail affiche « cannot assign ip address - no more free addresses from
// pool » — le pool IP du hotspot est ÉPUISÉ. Causes conjointes :
//
//  1. pool trop petit (un /24 = ~250 adresses partagées entre clients
//     PAYANTS et tous les appareils à portée qui réclament une IP AVANT
//     login — le portail captif exige une IP pour s'afficher) ;
//  2. « zombies » : RouterOS garde l'hôte (et son IP) indéfiniment quand
//     login-timeout n'est pas posé (défaut : aucun) ; côté DHCP, un bail
//     long brûle l'IP d'un appareil parti pour des heures ;
//  3. addresses-per-mac = 2 par défaut : un même appareil peut prendre
//     DEUX adresses.
//
// N°108 — BUG DE MENU RouterOS corrigé (constat production ProMax WIFI,
// l'extension « ne fonctionne pas ») : address-pool et addresses-per-mac
// sont des propriétés du SERVEUR hotspot (/ip hotspot) — PAS du profil
// (/ip hotspot profile n'a AUCUNE des deux). Le script N°97 lisait/écrivait
// les deux sur le profil : chaque get échouait silencieusement (on-error →
// chaîne vide), tout profil paraissait « sans pool », et le set de
// l'extension échouait à chaque fois — le pool n'a JAMAIS été étendu (seuls
// l'IP secondaire, l'entrée network et le NAT étaient posés, autour d'un
// pool orphelin « mikcloud-pool » référencé par personne). Le script corrige
// les menus ET étend le VRAI fournisseur d'adresses :
//
//   - DIAGNOSTIC (toujours, lecture seule) : pools (nom + ranges), serveurs
//     hotspot (8 champs : nom, profil, interface, login/idle/keepalive,
//     address-pool DU SERVEUR, addresses-per-mac), serveurs DHCP (4 champs :
//     nom, interface, pool, lease-time) et hôtes/sessions actives → le cloud
//     calcule la capacité (PoolCap : pools référencés par les serveurs
//     hotspot OU par le DHCP de leur interface — un serveur sans address-pool
//     s'appuie sur le DHCP du bridge, cas réel ProMax WIFI) et suit
//     l'occupation (PoolHosts, alimenté aussi par read_state). La liste
//     « profils » du N°97 est retirée : elle ne rapportait que des champs
//     vides (propriétés inexistantes sur ce menu) ;
//   - RECYCLAGE (payload recycle, défaut true côté endpoint console) :
//     login-timeout=5m, idle-timeout=10m, keepalive-timeout=2m et
//     addresses-per-mac=1 sur les serveurs hotspot, lease-time=10m sur les
//     serveurs DHCP des interfaces hotspot — libère les IP des zombies sans
//     toucher au subnet, zéro impact pour les sessions payantes (le cookie
//     hotspot re-connecte l'usager au réveil de son écran ; le solde du
//     voucher est intact) ;
//   - EXTENSION (payload extend, opt-in explicite du gérant) : par serveur
//     hotspot — (a) le serveur a un address-pool → le range dédié
//     10.77.0.10-10.77.7.254 (~2 037 IP) est AJOUTÉ à CE pool ; (b) sinon,
//     le serveur DHCP de la même interface a un pool → le range est ajouté
//     au pool DU DHCP (topologie ProMax WIFI : la capacité vient de là) ;
//     (c) sinon, pool dédié « mikcloud-pool » posé SUR LE SERVEUR. Dans
//     tous les cas : IP secondaire 10.77.0.1/21 sur l'interface hotspot
//     (ARP + passerelle), entrée /ip hotspot network masquerade, entrée
//     /ip dhcp-server network (gateway 10.77.0.1 — sans elle, le DHCP
//     n'offre pas proprement le nouveau range) et règle NAT
//     mikcloud-pool-nat (match src uniquement — aucune interface WAN à
//     deviner). Ménage inclus : le pool orphelin « mikcloud-pool » laissé
//     par le N°97 (créé mais jamais référencé) est retiré s'il ne sert
//     plus. Les clients connectés ne sont pas déconnectés : seules les
//     NOUVELLES attributions tirent du range étendu.
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

// PoolDoctorExtSubnet — réseau de l'extension (entrées hotspot network,
// dhcp-server network et NAT).
const PoolDoctorExtSubnet = "10.77.0.0/21"

// PoolDoctorExtGateway — IP secondaire posée sur l'interface hotspot
// (ARP + passerelle des clients du range étendu).
const PoolDoctorExtGateway = "10.77.0.1/21"

// PoolDoctorPoolName — nom du pool dédié posé sur un serveur hotspot sans
// address-pool NI DHCP sur son interface.
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
	leaseTO := sanitizeRosToken(plStr(cmd.Payload, "leaseTimeout"))
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
	// Passerelle nue (sans le /21) pour l'entrée dhcp-server network.
	extGwIP := extGateway
	if i := strings.IndexByte(extGateway, '/'); i > 0 {
		extGwIP = extGateway[:i]
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
	if leaseTO == "" {
		leaseTO = "10m"
	}

	var sb strings.Builder
	sb.WriteString(header(cmd))
	// mikClean — fonction LOCALE du script : remplace les séparateurs du
	// protocole de rapport (« | ; & = % ») par « _ » dans les valeurs
	// dynamiques (pattern du nettoyage caractère par caractère de
	// buildReadLog, factorisé ici car le docteur rapporte 3 listes).
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
	// ── Recyclage : timeouts + addresses-per-mac (SERVEUR /ip hotspot —
	// N°108 : jamais sur le profil, les propriétés n'y existent pas) et
	// lease-time des DHCP des interfaces hotspot. Trois écrits ISOLÉS :
	// un RouterOS ancien qui ignore addresses-per-mac ne doit pas faire
	// échouer les timeouts (et inversement) — la vérité fine repart dans
	// le rapport (les valeurs sont RELUES juste après).
	if recycle {
		sb.WriteString(`:do {
  /ip hotspot set [find] login-timeout=` + loginTO + ` idle-timeout=` + idleTO + ` keepalive-timeout=` + keepaliveTO + `
  :set mkRecycled "yes"
} on-error={ :log warning "mikcloud: recyclage timeouts hotspot impossible" }
:do {
  /ip hotspot set [find] addresses-per-mac=1
  :set mkRecycled "yes"
} on-error={ :log warning "mikcloud: addresses-per-mac inaccessible" }
:do {
  :foreach se in=[/ip hotspot find] do={
    :local sif ""
    :do { :set sif [:tostr [/ip hotspot get $se interface]] } on-error={}
    :if ([:len $sif] > 0) do={
      :do { /ip dhcp-server set [find where interface=$sif] lease-time=` + leaseTO + ` } on-error={}
    }
  }
  :set mkRecycled "yes"
} on-error={ :log warning "mikcloud: recyclage lease-time DHCP impossible" }
`)
	}
	// ── Extension : le range dédié rejoint le VRAI fournisseur d'adresses
	// de chaque serveur hotspot — (a) son address-pool, (b) sinon le pool
	// du serveur DHCP de la même interface (cas ProMax WIFI), (c) sinon un
	// pool dédié posé SUR le serveur. Puis passerelle/network/NAT.
	if extend {
		sb.WriteString(`:do {
  :foreach se in=[/ip hotspot find] do={
    :local sif ""
    :do { :set sif [:tostr [/ip hotspot get $se interface]] } on-error={}
    :local pap ""
    :do { :set pap [:tostr [/ip hotspot get $se address-pool]] } on-error={}
    :if ([:len $pap] > 0 && $pap != "none" && $pap != "(unknown)") do={
      :local pid [/ip pool find name=$pap]
      :if ([:len $pid] > 0) do={
        :local pr ""
        :do { :set pr [:tostr [/ip pool get $pid ranges]] } on-error={}
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
      :local dp ""
      :do {
        :foreach de in=[/ip dhcp-server find where interface=$sif] do={
          :local dpp ""
          :do { :set dpp [:tostr [/ip dhcp-server get $de address-pool]] } on-error={}
          :if ([:len $dpp] > 0 && $dpp != "none" && $dpp != "static" && [:len $dp] = 0) do={ :set dp $dpp }
        }
      } on-error={}
      :if ([:len $dp] > 0) do={
        :local pid [/ip pool find name=$dp]
        :if ([:len $pid] > 0) do={
          :local pr ""
          :do { :set pr [:tostr [/ip pool get $pid ranges]] } on-error={}
          :if ([:typeof [:find $pr "` + extRange + `"]] = "nil") do={
            :do {
              /ip pool set $pid ranges=($pr . "," . "` + extRange + `")
              :set mkExtended "yes"
            } on-error={ :log warning "mikcloud: extension du pool DHCP impossible" }
          } else={
            :set mkExtended "yes"
          }
        }
      } else={
        :if ([:len [/ip pool find name="` + PoolDoctorPoolName + `"]] = 0) do={
          :do { /ip pool add name="` + PoolDoctorPoolName + `" ranges="` + extRange + `" } on-error={}
        }
        :do {
          /ip hotspot set $se address-pool="` + PoolDoctorPoolName + `"
          :set mkExtended "yes"
        } on-error={ :log warning "mikcloud: pool dedie non posable sur le serveur" }
      }
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
  :if ([:len [/ip dhcp-server network find where address="` + extSubnet + `"]] = 0) do={
    /ip dhcp-server network add address="` + extSubnet + `" gateway=` + extGwIP + ` comment="mikcloud-pool"
  }
} on-error={ :log warning "mikcloud: entree dhcp network impossible" }
:do {
  :if ([:len [/ip firewall nat find where comment="mikcloud-pool-nat"]] = 0) do={
    /ip firewall nat add chain=srcnat src-address="` + extSubnet + `" action=masquerade comment="mikcloud-pool-nat"
  }
} on-error={ :log warning "mikcloud: regle NAT pool impossible" }
:do {
  :local mkRef 0
  :foreach se in=[/ip hotspot find] do={
    :local ap ""
    :do { :set ap [:tostr [/ip hotspot get $se address-pool]] } on-error={}
    :if ($ap = "` + PoolDoctorPoolName + `") do={ :set mkRef 1 }
  }
  :foreach de in=[/ip dhcp-server find] do={
    :local dp2 ""
    :do { :set dp2 [:tostr [/ip dhcp-server get $de address-pool]] } on-error={}
    :if ($dp2 = "` + PoolDoctorPoolName + `") do={ :set mkRef 1 }
  }
  :if ($mkRef = 0) do={
    :local mkOrph [/ip pool find name="` + PoolDoctorPoolName + `"]
    :if ([:len $mkOrph] > 0) do={
      :do { /ip pool remove $mkOrph } on-error={ :log warning "mikcloud: retrait du pool orphelin impossible" }
    }
  }
} on-error={}
`)
	}
	// ── Diagnostic : pools (ranges APRÈS extension), serveurs hotspot
	// (8 champs — address-pool et addresses-per-mac RELUS sur le serveur :
	// la vérité de ce qui a VRAIMENT collé), serveurs DHCP (4 champs, avec
	// lease-time). Plus de liste « profils » : ces propriétés n'existent
	// pas sur /ip hotspot profile (le N°97 y lisait des champs vides).
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
    :local sap ""
    :do { :set sap [:tostr [/ip hotspot get $se address-pool]] } on-error={}
    :local sam ""
    :do { :set sam [:tostr [/ip hotspot get $se addresses-per-mac]] } on-error={}
    :set mkSrv ($mkSrv . [$mikClean $sn] . "|" . [$mikClean $sp] . "|" . [$mikClean $si] . "|" . [$mikClean $slt] . "|" . [$mikClean $sid] . "|" . [$mikClean $skt] . "|" . [$mikClean $sap] . "|" . [$mikClean $sam] . ";")
  }
} on-error={}
:local mkDhcp ""
:do {
  :foreach de in=[/ip dhcp-server find] do={
    :local dn [:tostr [/ip dhcp-server get $de name]]
    :local di ""
    :do { :set di [:tostr [/ip dhcp-server get $de interface]] } on-error={}
    :local dp ""
    :do { :set dp [:tostr [/ip dhcp-server get $de address-pool]] } on-error={}
    :local dl ""
    :do { :set dl [:tostr [/ip dhcp-server get $de lease-time]] } on-error={}
    :if ([:len $dn] > 0) do={
      :set mkDhcp ($mkDhcp . [$mikClean $dn] . "|" . [$mikClean $di] . "|" . [$mikClean $dp] . "|" . [$mikClean $dl] . ";")
    }
  }
} on-error={}
:local mkData ("cmd=` + cmd.ID + `&status=ok&recycled=". $mkRecycled ."&extended=". $mkExtended ."&hosts=". $mkHosts ."&active=". $mkActive ."&pools=". $mkPools ."&servers=". $mkSrv ."&dhcp=". $mkDhcp)
`)
	sb.WriteString(`/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=($mkData) output=none` + "\n")
	return sb.String()
}
