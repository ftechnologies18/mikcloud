// Package model — types partagés MikCloud, alignés sur le contrat API (voir worklog.md).
package model

import "time"

// ---------------------------------------------------------------------------
// Types métier (réponses JSON strictement conformes au contrat TS)
// ---------------------------------------------------------------------------

// AccountMainID — identifiant littéral du compte principal (plateforme). Le
// compte principal porte TOUJOURS cet ID (migrations déterministes) : il hérite
// des données de l'ère mono-tenant et ne peut pas être désactivé.
const AccountMainID = "acc-main"

// Account — compte client SaaS (isolation multi-tenant). Chaque entité métier
// porte un AccountID : un compte ne voit et ne modifie que ses données.

// Account — compte client SaaS (isolation multi-tenant). Chaque entité métier
// porte un AccountID : un compte ne voit et ne modifie que ses données.
type Account struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"` // active | disabled
	CreatedAt string `json:"createdAt"`
	// Contact propriétaire (signup) — support WhatsApp/email + segmentation
	// géographique (devise/timezone automatiques, rapports commerciaux).
	Email   string `json:"email,omitempty"`
	Phone   string `json:"phone,omitempty"`   // WhatsApp de préférence, format E.164 sans +
	Country string `json:"country,omitempty"` // code ISO 3166-1 alpha-2 (CI, SN, NG…) ou "other"
	City    string `json:"city,omitempty"`
	// N°98 — usage du compte : « hotspot » (réseaux publics payants — le
	// produit historique) ou « homenet » (réseaux privés domestiques).
	// Le champ s'appelle usage et JAMAIS mode : Router.Mode désigne déjà le
	// mode de CONNEXION au routeur (agent/API) — collision de vocabulaire
	// évitée dès la conception. Défaut « hotspot » : tout le parc existant
	// reste sur le produit historique (colonne posée par ALTER idempotent,
	// zéro changement visible — Phase 1 = plomberie seulement).
	Usage string `json:"usage,omitempty"`
}

// Router — équipement MikroTik géré (simulé, réel ou agent). Password non exposé dans l'API.

// Router — équipement MikroTik géré (simulé, réel ou agent). Password non exposé dans l'API.
type Router struct {
	ID             string `json:"id"`
	AccountID      string `json:"accountId"`
	Name           string `json:"name"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	Password       string `json:"password,omitempty"`
	Mode           string `json:"mode"`   // simulated | real | agent
	Status         string `json:"status"` // online | offline
	Version        string `json:"version"`
	UptimeSec      int64  `json:"uptimeSec"`
	CPULoad        int    `json:"cpuLoad"`
	HotspotUsers   int    `json:"hotspotUsers"`
	ActiveSessions int    `json:"activeSessions"`
	// UnknownOnRouter — compteur VOLATIL (non persisté, recalculé à chaque
	// read_state) d'utilisateurs présents sur le routeur mais inconnus du
	// cloud. Alimenté par applyReadState : utile quand l'import automatique
	// est désactivé (les comptes hors MikCloud restent listés ici pour
	// adoption manuelle). omitempty → absent du JSON tant que 0.
	UnknownOnRouter int    `json:"unknownOnRouter,omitempty"`
	CreatedAt       string `json:"createdAt"`
	// HotspotLoginUrl — page de login du portail captive MikroTik (ex.
	// http://10.5.50.1/login). Utilisée par les QR codes des vouchers imprimés :
	// le QR encode {url}?username=CODE&password=PASS → connexion en 1 scan.
	// Vide → le QR contient simplement « CODE / PASS ».
	HotspotLoginUrl string `json:"hotspotLoginUrl,omitempty"`
	// Mode agent (HTTP-poll sortant) : le token n'est JAMAIS stocké en clair.
	AgentTokenHash string `json:"agentTokenHash,omitempty"`
	TokenPreview   string `json:"tokenPreview,omitempty"`
	LastSeen       string `json:"lastSeen,omitempty"`
	// P1 (audit Mikhmon) — F8 : status étendu (board, disque). Colonnes
	// créées dès la vague P0 (migrations idempotentes pg.go).
	BoardName  string `json:"boardName,omitempty"`
	FreeHddMb  int    `json:"freeHddMb,omitempty"`
	TotalHddMb int    `json:"totalHddMb,omitempty"`
	// Sécurité S6 — détection d'identité routeur dupliquée : vrai quand
	// l'empreinte (identity + modèle RouterOS) déclarée par l'agent est déjà
	// portée par un routeur ACTIF d'un autre compte (fenêtre 24 h). L'agent
	// est alors refusé (409 au register, aucune commande au check-in) tant
	// que le conflit persiste ; levée automatique dès que le porteur disparaît
	// ou dort plus de 24 h. Jamais exposé au client de la console : c'est un
	// signal anti-abus interne (le client voit « hors ligne »).
	IdentityConflict bool `json:"identityConflict,omitempty"`
	// N°29 — signature de la configuration walled-garden d'inscription
	// publique DÉJÀ APPLIQUÉE avec succès sur ce routeur (hash des domaines
	// page+API). Vide → rien d'appliqué (ou dernier échec) : le check-in
	// suivant re-file la commande walled_garden. Posée au retour « ok » de la
	// commande (handleAgentResult) — jamais à la mise en file : un échec est
	// retenté automatiquement au check-in suivant.
	WalledGardenSig string `json:"walledGardenSig,omitempty"`
	// N°49 — horodatage (RFC3339) de la DERNIÈRE application confirmée du
	// walled-garden sur ce routeur. Rend le walled-garden AUTO-RÉPARANT :
	// posé avec la signature au retour « ok » de la commande, il sert de
	// base au re-file périodique (walledGardenRefresh) — une liste vidée
	// localement (ménage Mikhmon, restauration, ajout manuel partiel,
	// constat prod CyberSC 2026-09-06 : règles DNS posées mais règles page
	// absentes) est recréée au plus tard 6 h après, sans intervention.
	// Vide + sig posée = état antérieur au N°49 : considéré non frais →
	// re-file automatique au premier check-in suivant (réparation des
	// routeurs déjà en ligne, zéro action humaine).
	WalledGardenAppliedAt string `json:"walledGardenAppliedAt,omitempty"`
	// N°35 — signature du PORTAIL CAPTIF déjà déployé avec succès sur ce
	// routeur (hash de l'ensemble des fichiers personnalisés login.html,
	// status.html, assets). Vide → rien déployé (ou dernier échec) : le
	// check-in suivant re-file la commande hotspot_files. Posée au retour
	// « ok » uniquement. Pattern identique à WalledGardenSig : un échec est
	// retenté automatiquement, un changement de config (branding, offres,
	// textes) re-file automatiquement — le gérant change sa config dans la
	// console, l'agent re-déploie tout seul au prochain check-in (≤ 45 s).
	HotspotFilesSig string `json:"hotspotFilesSig,omitempty"`
	// N°75 — veille adaptative : intervalle ACTUEL (secondes) du scheduler
	// MikCloud sur ce routeur. 0 = état antérieur au N°75 (l'installation
	// pose 45 s — traité comme 45). Le cloud décide du pas à chaque
	// check-in : 45 s quand le routeur est « sous attention » (console
	// ouverte, commandes en jeu, invité sur le portail), sinon la veille
	// (180 s) — la capacité bande passante passe de ~100 à ~350-450
	// routeurs sur le plan gratuit. Posé au retour « ok » de la commande
	// scheduler_set, jamais à la mise en file.
	SchedulerSec int `json:"schedulerSec,omitempty"`
	// N°77 — veilleur d'invités : vrai quand le scheduler mikcloud-watch
	// (check-in 20 s pendant qu'un hôte non autorisé est présent — un
	// invité est SUR le portail, claim imminent) est CONFIRMÉ déployé sur
	// ce routeur. Faux = routeur antérieur au N°77 ou dernier déploiement
	// échoué → le check-in suivant re-file watcher_ensure (pattern
	// walled-garden : auto-réparant, posé au retour « ok » uniquement).
	// Constat production N°77 : la veille N°75 rendait le PREMIER claim
	// d'un routeur endormi lent (45 s → 1-2 min d'attente — un invité en
	// salle n'attend pas) ; le veilleur restaure un check-in ≤ 20 s
	// PENDANT la fenêtre invité sans coût idle (0 octet émis sans invité).
	WatcherOK bool `json:"watcherOK,omitempty"`
	// N°80 — SafeWiFi (protection DNS du WiFi public) : niveau de filtrage
	// choisi par le gérant pour ce site. "" = état antérieur au N°80
	// (traité comme "off" — aucun filtrage, AUCUNE commande filée : un
	// routeur dont le gérant n'ouvre jamais la carte ne consomme rien,
	// l'économie de veille N°75 reste entière).
	//   off      : aucune protection ;
	//   threats  : malwares, phishing, arnaques (résolveur Quad9) ;
	//   family   : + contenus adultes, publicités (AdGuard Family).
	SafeWifiLevel string `json:"safeWifiLevel,omitempty"`
	// N°80 — signature de la config SafeWiFi DÉJÀ APPLIQUÉE avec succès
	// sur ce routeur (hash du niveau + sel de version des règles).
	// Vide → rien d'appliqué (ou dernier échec) : le check-in suivant
	// re-file la commande safewifi. Posée au retour « ok » VÉRIFIÉ — le
	// re-file la commande safewifi. Posée au retour « ok » VÉRIFIÉ — le
	// routeur échoe le compte d'objets marqués mikcloud-safewifi présents
	// après application ET, depuis le N°95, leur disposition réelle en
	// table NAT (layout RRDD — boucliers pré-auth au-dessus des dst-nat ;
	// vérité routeur, pattern scheduler_set N°75 ; N°93 : 4 règles NAT +
	// liste DoH v4 + 2 règles FILTER par serveur hotspot rapporté, 0
	// sinon).
	SafeWifiSig string `json:"safeWifiSig,omitempty"`
	// N°80 — horodatage (RFC3339) de la dernière application confirmée :
	// auto-réparation périodique (une règle effacée localement par un
	// ménage ou une restauration de backup est recréée au plus tard
	// safeWifiRefresh plus tard). Pattern walled-garden N°49.
	SafeWifiAppliedAt string `json:"safeWifiAppliedAt,omitempty"`

	// N°81 — Shield (bouclier réseau du WiFi public) : état du blindage
	// choisi par le gérant pour ce site. "" = état antérieur au N°81
	// (traité comme "off" — aucune commande filée : un routeur dont le
	// gérant n'ouvre jamais la carte ne consomme rien, pattern N°80).
	//   off : aucun blindage ;
	//   on  : administration du routeur (winbox, ssh, telnet, api) et
	//         vecteurs malveillants (SMB/NetBIOS, connexions invalides)
	//         bloqués pour les clients du WiFi public.
	ShieldLevel string `json:"shieldLevel,omitempty"`
	// N°81 — signature de la config Shield DÉJÀ APPLIQUÉE avec succès sur
	// ce routeur (hash du niveau + sel de version des règles filter).
	// Vide → rien d'appliqué : le check-in suivant re-file la commande
	// shield. Posée au retour « ok » VÉRIFIÉ — le routeur échoe le compte
	// de règles marquées mikcloud-shield ET le nombre de serveurs hotspots
	// trouvés (5 règles par hotspot ; vérité routeur, pattern N°80).
	ShieldSig string `json:"shieldSig,omitempty"`
	// N°81 — horodatage (RFC3339) de la dernière application confirmée :
	// auto-réparation périodique (règles recréées au plus tard
	// shieldRefresh plus tard). Pattern walled-garden N°49.
	ShieldAppliedAt string `json:"shieldAppliedAt,omitempty"`

	// N°82 — FamilyGuard (couvre-feu internet du WiFi public) : fenêtre
	// horaire programmée par le gérant, forme canonique
	// "<enabled>|<HH:MM>|<HH:MM>|<1111111>" (ex. "1|22:00|06:00|1111111"
	// = actif tous les soirs de 22:00 à 06:00 ; days = lundi→dimanche).
	// "" = jamais utilisé → AUCUNE commande filée : un routeur dont le
	// gérant n'ouvre jamais la carte ne consomme rien, l'économie de
	// veille N°75 reste entière (pattern N°80/N°81).
	FamilyGuardSpec string `json:"familyGuardSpec,omitempty"`
	// N°82 — signature de la config FamilyGuard DÉJÀ APPLIQUÉE avec
	// succès sur ce routeur (hash du spec + de l'ÉTAT désiré au moment
	// de l'application — dans la fenêtre ou non : la signature change à
	// chaque frontière de fenêtre, le check-in suivant re-file la
	// bascule). Posée au retour « ok » VÉRIFIÉ (règles marquées ==
	// 1 × hotspots rapportés en couvre-feu, 0 sinon).
	FamilyGuardSig string `json:"familyGuardSig,omitempty"`
	// N°82 — horodatage (RFC3339) de la dernière application confirmée :
	// auto-réparation périodique (pattern walled-garden N°49).
	FamilyGuardAppliedAt string `json:"familyGuardAppliedAt,omitempty"`

	// N°88 — AntiVPN (bloque-VPN du WiFi public) : état du blocage des
	// tunnels VPN choisi par le gérant pour ce site. "" = état antérieur
	// au N°88 (traité comme "off" — aucune commande filée : un routeur
	// dont le gérant n'ouvre jamais la carte ne consomme rien, pattern
	// N°80/N°81/N°82).
	//   off : aucun blocage ;
	//   on  : VPN et tunnels standards (GRE, IPsec/IKEv2, L2TP, PPTP,
	//         OpenVPN, WireGuard, WARP, Tor) coupés pour les clients.
	AntiVpnLevel string `json:"antiVpnLevel,omitempty"`
	// N°88 — signature de la config AntiVPN DÉJÀ APPLIQUÉE avec succès
	// sur ce routeur (hash du niveau + sel de version des règles).
	// Vide → rien d'appliqué (ou dernier échec) : le check-in suivant
	// re-file la commande antivpn. Posée au retour « ok » VÉRIFIÉ — le
	// routeur échoe le compte de règles marquées mikcloud-antivpn
	// (4 par serveur hotspot rapporté, 0 sinon ; vérité routeur,
	// pattern Shield N°81).
	AntiVpnSig string `json:"antiVpnSig,omitempty"`
	// N°88 — horodatage (RFC3339) de la dernière application confirmée :
	// auto-réparation périodique (pattern walled-garden N°49).
	AntiVpnAppliedAt string `json:"antiVpnAppliedAt,omitempty"`

	// N°97 — Docteur pool IP (épuisement « no more free addresses from
	// pool » aux heures de pointe) :
	// PoolCap — capacité TOTALE (adresses) des pools référencés par les
	// profils hotspot, calculée par le cloud depuis les ranges rapportés
	// (pool_doctor). 0 = jamais diagnostiqué → l'occupation n'est ni
	// affichée ni alertée (honnêteté : sans capacité connue, aucun
	// pourcentage inventé).
	PoolCap int `json:"poolCap,omitempty"`
	// PoolHosts — hôtes tenant une IP (authentifiés + zombies) —
	// rafraîchi à chaque read_state (paramètre hosts), posé aussi par
	// le docteur. L'écart PoolHosts - ActiveSessions = IP zombies.
	PoolHosts int `json:"poolHosts,omitempty"`
	// PoolRanges — ranges du pool actif (affichage console, « ; » entre
	// pools, « , » entre ranges) — vérité routeur du dernier diagnostic.
	PoolRanges string `json:"poolRanges,omitempty"`
	// PoolDoctorAt — RFC3339 du dernier diagnostic pool confirmé (auto-
	// diagnostiqueur du check-in, re-filé passé PoolDoctorRefresh).
	PoolDoctorAt string `json:"poolDoctorAt,omitempty"`
	// N°99 — auto-réparation du pool (opt-in gérant, switch de la carte
	// « Pool d'adresses IP ») : à la transition d'alerte high/full le
	// moniteur marque PoolAutoPending, et le check-in suivant enfile le
	// recyclage des IP zombies SANS geste humain (jamais l'extension —
	// geste topologique à confirmation explicite).
	PoolAuto bool `json:"poolAuto,omitempty"`
	// PoolAutoPending — auto-réparation demandée par le moniteur, en
	// attente du check-in qui la filera (consommé au filage ; reste posé
	// tant qu'une commande pool_doctor est en file ou en vol).
	PoolAutoPending bool `json:"poolAutoPending,omitempty"`

	// N°101 — pause dîner HomeNet : signature de l'ENSEMBLE pause DÉJÀ
	// APPLIQUÉ avec succès sur ce routeur (hash de la liste triée des
	// MAC en pause + sel de version dp-v1). Vide → rien d'appliqué (ou
	// dernier échec) : le check-in suivant re-file la commande
	// device_pause dès que la signature désirée diffère (pause posée,
	// levée, expirée, appareil renommé dans le registre n'y compte pas —
	// seules les MAC comptent). Posée au retour « ok » VÉRIFIÉ — le
	// routeur échoe le compte de règles marquées mikcloud-pause APRÈS
	// application, la signature n'est posée que si rules == len(macs)
	// du payload ET si la version envoyée est TOUJOURS la version
	// désirée (un parent qui change d'avis pendant le vol ne doit pas
	// voir figé un état périmé — pattern SafeWiFi N°80).
	PauseSig string `json:"pauseSig,omitempty"`

	// N°103 — Qualité de ligne (mesure passive du débit FAI) :
	// WanIface est l'interface WAN DÉTECTÉE par le script read_state
	// (route par défaut active → « reachable via <iface> »). Lecture
	// seule côté console — c'est la vérité routeur, jamais une saisie.
	// Vide = pas encore détecté (premier read_state post-N°103, routeur
	// sans route par défaut publiée) : les agrégats existent mais
	// l'endpoint line-quality ne rattachera AUCUNE interface à la ligne
	// (honnêteté : aucun WAN deviné, miroir pooldoctor N°97).
	WanIface string `json:"wanIface,omitempty"`
	// N°103 — capacité ligne DÉCLARÉE par le gérant (bits/s, 0 = non
	// renseignée) : « Orange CI 110M/20M » pour CE site. Chaque client
	// MikCloud a un FAI et un forfait différents — JAMAIS de valeur
	// globale. Prime sur la mesure passive (étiquette vs enveloppe
	// observée : c'est l'étiquette qui borne le burst) ; la mesure
	// l'affine (une ligne qui ne rend jamais plus de 96M se découvre au
	// premier jour d'agrégats, même si l'offre dit 110M).
	LineDownBps int64 `json:"lineDownBps,omitempty"`
	LineUpBps   int64 `json:"lineUpBps,omitempty"`

	// N°104 — QoS Manager : plafond AGRÉGAT du hotspot, automatisé.
	// QoSEnabled — opt-in du gérant ; la file mikcloud-qos converge au
	// check-in (pattern walled_garden : sig posée au retour « ok »
	// VÉRIFIÉ, re-file tant qu'elle manque, auto-réparation 6 h).
	// Désactivé après avoir été appliqué : la RETRAITE converge de même
	// (QoSAppliedAt non vide = une file existe à retirer).
	QoSEnabled bool `json:"qosEnabled,omitempty"`
	// QoSTarget — sous-réseau hotspot bridé (CIDR IPv4, ex.
	// 192.168.10.0/24). La file simple cible CE réseau : le LAN privé
	// garde sa part par construction, l'ordre des files n'importe plus.
	QoSTarget string `json:"qosTarget,omitempty"`
	// QoSMaxUpBps/QoSMaxDownBps — limites APPLIQUÉES (max-limit
	// upload/download, bits/s). Le burst/threshold sont DÉRIVÉS
	// déterministement (burst = capacité ≈ max×20/19, seuil = 80 % du
	// max) : seules les limites persistent, tout est recalculé à
	// l'identique au filage ET dans les tests.
	QoSMaxUpBps   int64 `json:"qosMaxUpBps,omitempty"`
	QoSMaxDownBps int64 `json:"qosMaxDownBps,omitempty"`
	// QoSSig — signature de la config DÉJÀ APPLIQUÉE (hash
	// cible+limites+sel de version). Posée au retour « ok » VÉRIFIÉ —
	// la relecture routeur (target|max-limit|queue|disabled) doit
	// correspondre bit à bit au payload (les deux côtés normalisent en
	// bps : le formatage RouterOS « 17M » ou « 17000000 » ne compte
	// plus). Vide → re-file au check-in suivant.
	QoSSig string `json:"qosSig,omitempty"`
	// QoSAppliedAt — RFC3339 de la dernière application confirmée :
	// auto-réparation périodique (qosRefresh) ET marqueur de retrait
	// (non vide + désactivé = une file à retirer).
	QoSAppliedAt string `json:"qosAppliedAt,omitempty"`
}

// SchedulerSecEffective — pas de scheduler connu du routeur (N°75). 0 =
// état antérieur au correctif : l'installation posait 45 s.

// SchedulerSecEffective — pas de scheduler connu du routeur (N°75). 0 =
// état antérieur au correctif : l'installation posait 45 s.
func (r *Router) SchedulerSecEffective() int {
	if r.SchedulerSec <= 0 {
		return 45
	}
	return r.SchedulerSec
}

// EffectiveOfflineAfter — N°75 — seuil « hors ligne » effectif d'un routeur
// agent : le réglage du compte (OfflineAfterSec, défaut 135 s = 3 × 45 s) OU
// 3 × son pas de scheduler, LE PLUS GRAND des deux. Un routeur en veille
// (180 s) serait sinon marqué hors ligne entre deux check-ins — fausses
// alertes « Routeur hors ligne » à chaque sieste. Partagé par le moniteur
// de notifications (package notify) et le sync-status (package api) : la
// formule vit ici, avec le modèle, pour éviter un import croisé.

// EffectiveOfflineAfter — N°75 — seuil « hors ligne » effectif d'un routeur
// agent : le réglage du compte (OfflineAfterSec, défaut 135 s = 3 × 45 s) OU
// 3 × son pas de scheduler, LE PLUS GRAND des deux. Un routeur en veille
// (180 s) serait sinon marqué hors ligne entre deux check-ins — fausses
// alertes « Routeur hors ligne » à chaque sieste. Partagé par le moniteur
// de notifications (package notify) et le sync-status (package api) : la
// formule vit ici, avec le modèle, pour éviter un import croisé.
func (r *Router) EffectiveOfflineAfter(cfgOfflineSec int) time.Duration {
	if cfgOfflineSec <= 0 {
		cfgOfflineSec = 135
	}
	effective := cfgOfflineSec
	if v := 3 * r.SchedulerSecEffective(); v > effective {
		effective = v
	}
	return time.Duration(effective) * time.Second
}

// Profile — profil hotspot (débit, durée, prix, validité).

// Profile — profil hotspot (débit, durée, prix, validité).
type Profile struct {
	ID                string `json:"id"`
	AccountID         string `json:"accountId"`
	Name              string `json:"name"`
	RateLimit         string `json:"rateLimit"`
	SessionTimeoutMin int    `json:"sessionTimeoutMin"`
	SharedUsers       int    `json:"sharedUsers"`
	ValidityDays      int    `json:"validityDays"`
	Price             int    `json:"price"`
	DataQuotaMb       int    `json:"dataQuotaMb"`
	CreatedAt         string `json:"createdAt"`
	// P0 (audit Mikhmon) — expiration cloud (F1).
	ExpMode        string `json:"expMode"`        // "none" (parité Mikhmon « None ») | "notify" (défaut) | "remove"
	GracePeriodMin int    `json:"gracePeriodMin"` // 0 = immédiat (borne 43200)
	LockUser       bool   `json:"lockUser"`       // verrouiller : 1 session à la fois
	// P2 (audit Mikhmon) — marge (F13) : prix de vente affiché (0 = même prix que Price).
	SellingPrice int `json:"sellingPrice"`
	// v2 — anti-partage : chaque utilisateur du profil est verrouillé au PREMIER
	// appareil qui se connecte avec son identifiant (liaison MAC, appliquée par
	// le script on-login du profil sur le routeur ; le MAC est mémorisé dans le
	// commentaire routeur sous la marque « mikcloud_lock: », jamais renvoyé au
	// cloud). Les autres appareils sont déconnectés à la connexion.
	LockFirstDevice bool `json:"lockFirstDevice"`
	// Parité Mikhmon (profil User Profile) : Address Pool et Parent Queue —
	// noms RouterOS transmis tels quels au routeur ("" = none/absent).
	// address-pool : pool IP du routeur (/ip pool) servi au client hotspot ;
	// parent-queue : queue simple (/queue simple) héritée par les utilisateurs.
	AddressPool string `json:"addressPool"`
	ParentQueue string `json:"parentQueue"`
	// Parité Mikhmon : validité au format RouterOS [wdhm] (ex. 5h30m, 4w3d).
	// ValidityMin = source de vérité fine (minutes, 0 = hériter ValidityDays × 1440
	// pour la compatibilité contrat V2 / données existantes). Utiliser TOUJOURS
	// ValidityMinutes() pour calculer une expiration.
	ValidityMin int `json:"validityMin"`
}

// ValidityMinutes — durée de validité effective du profil en minutes.
// Extension parité Mikhmon : ValidityMin (> 0) prime sur ValidityDays
// (champ historique du contrat V2, conservé pour rétro-compatibilité).

// ValidityMinutes — durée de validité effective du profil en minutes.
// Extension parité Mikhmon : ValidityMin (> 0) prime sur ValidityDays
// (champ historique du contrat V2, conservé pour rétro-compatibilité).
func (p Profile) ValidityMinutes() int {
	if p.ValidityMin > 0 {
		return p.ValidityMin
	}
	return p.ValidityDays * 1440
}

// HotspotUser — utilisateur hotspot régulier ou voucher.

// HotspotUser — utilisateur hotspot régulier ou voucher.
type HotspotUser struct {
	ID           string `json:"id"`
	AccountID    string `json:"accountId"`
	Kind         string `json:"kind"` // regular | voucher
	Username     string `json:"username"`
	Password     string `json:"password"`
	ProfileID    string `json:"profileId"`
	ProfileName  string `json:"profileName"`
	RouterID     string `json:"routerId"`
	RouterName   string `json:"routerName"`
	Status       string `json:"status"` // active | used | expired | disabled
	BatchID      string `json:"batchId"`
	ResellerID   string `json:"resellerId"`
	ResellerName string `json:"resellerName"`
	Comment      string `json:"comment"`
	// Sémantique RouterOS officielle (help.mikrotik.com — HotSpot) — point de
	// vue du ROUTEUR : bytes-in = bytes UPLOADÉS par le client, bytes-out =
	// bytes TÉLÉCHARGÉS. Compteurs transportés BRUTS (somme invariante pour les
	// quotas) ; l'étiquetage client est verrouillé côté front
	// (frontend/src/lib/hotspot/traffic-semantics.ts).
	BytesIn       int64  `json:"bytesIn"`
	BytesOut      int64  `json:"bytesOut"`
	UptimeUsedSec int64  `json:"uptimeUsedSec"`
	CreatedAt     string `json:"createdAt"`
	ExpiresAt     string `json:"expiresAt"`
	UsedAt        string `json:"usedAt"`
	Price         int    `json:"price"`
	// P0/P2 (audit Mikhmon) — marge (F13) : prix de vente copié du profil à la
	// génération ({{price}} du voucher = sellingPrice || price).
	SellingPrice int `json:"sellingPrice"`
	// P0 (audit Mikhmon) — F1 : false tant que l'expiration n'a pas été
	// appliquée au routeur (user_remove / user_set disabled — voir enforceExpired).
	Enforced bool `json:"enforced"`
	// DataQuotaMb — quota de données par voucher appliqué sur le routeur
	// (/ip hotspot user add limit-bytes-total=…, exprimé en Mo ; 0 = illimité
	// dans la limite de la validité). Ex. « 5 Go = 500 F » → DataQuotaMb 5120.
	DataQuotaMb int64 `json:"dataQuotaMb"`
	// Parité Mikhmon — Time Limit PAR LOT (limit-uptime RouterOS) : quota de
	// temps CUMULÉ propre au voucher, surchargé à la génération (minutes ;
	// 0 = hériter du sessionTimeoutMin du profil à la génération).
	TimeLimitMin int64 `json:"timeLimitMin"`
	// N°8 — Mode Vente : remise effective du voucher au client par le
	// revendeur (traçabilité anti-vol). Vide = encore en stock.
	SoldAt  string `json:"soldAt,omitempty"`  // RFC3339
	SoldVia string `json:"soldVia,omitempty"` // "sell_mode" (app revendeur)
	// N°19 — dépôt-vente : ticket attribué À CRÉDIT (prise non payée) ;
	// sa remise au client crée une créance (Transaction « debt »).
	CreditSale bool `json:"creditSale,omitempty"`
	// N (rapprochement doux) — true quand le dernier read_state du routeur
	// n'a PAS listé cet utilisateur alors qu'il devrait y être (supprimé
	// directement dans Winbox, commande échouée…). Le cloud le conserve
	// (registre durable) et propose une resynchronisation : recréer sur le
	// routeur ou l'oublier. Jamais marqué pour les statuts used/expired
	// (absence attendue) ni les comptes récents (grâce 2 min, commandes
	// en file).
	MissingOnRouter bool `json:"missingOnRouter,omitempty"`
	// Disabled — miroir du statut STOCKÉ (désactivation manuelle) : le champ
	// Status sérialisé par les listes porte le statut RÉSOLU (ResolvedStatus),
	// où une expiration calculée peut masquer « disabled ». L'UI s'y réfère
	// pour le bouton activer/désactiver. Calculé à la lecture, jamais persisté.
	Disabled bool `json:"disabled,omitempty"`
}

// Session — session hotspot active.

// Session — session hotspot active.
type Session struct {
	ID          string `json:"id"`
	AccountID   string `json:"accountId"`
	UserID      string `json:"userId"`
	Username    string `json:"username"`
	ProfileName string `json:"profileName"`
	RouterID    string `json:"routerId"`
	RouterName  string `json:"routerName"`
	IP          string `json:"ip"`
	MAC         string `json:"mac"`
	StartedAt   string `json:"startedAt"`
	UptimeSec   int64  `json:"uptimeSec"`
	// RouterOS : bytes-in = uploadé, bytes-out = téléchargé (POV routeur —
	// voir le commentaire HotspotUser et traffic-semantics.ts côté front).
	BytesIn  int64 `json:"bytesIn"`
	BytesOut int64 `json:"bytesOut"`
}

// Reseller — revendeur avec portefeuille.

// Reseller — revendeur avec portefeuille.
type Reseller struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	Name      string `json:"name"`
	Username  string `json:"username"`
	Phone     string `json:"phone"`
	Credit    int    `json:"credit"`
	// N°19 — modes de paiement : « prepaid » (historique : crédit débité
	// à la prise de stock) ou « deposit » (dépôt-vente : il vend puis
	// verse — créance née à la remise, bornée par le plafond).
	PaymentMode  string `json:"paymentMode"` // prepaid | deposit
	DebtCeiling  int    `json:"debtCeiling"`
	VouchersSold int    `json:"vouchersSold"`
	Revenue      int    `json:"revenue"`
	Status       string `json:"status"` // active | disabled
	CreatedAt    string `json:"createdAt"`
	// N°8 — Mode Vente : PIN (4-6 chiffres) pour l'app PWA du revendeur.
	// Bcrypt ; vide = connexion Mode Vente interdite. Persisté dans db.json
	// (le store sérialise via ces mêmes tags) mais TOUJOURS vidé par
	// sanitizeReseller avant toute réponse API.
	PinHash string `json:"pinHash,omitempty"`
	// N°66 — limite d'appareils simultanés (Mode Vente) : nombre maximal
	// de sessions PIN actives en même temps pour ce compte. 0 = illimité
	// (comportement historique, token stateless sans registre). N ≥ 1 : le
	// login enregistre une session (jti embarqué dans le JWT) et déconnecte
	// l'appareil connecté depuis le plus longtemps au-delà de la limite.
	MaxDevices int `json:"maxDevices"`
}

// SellSession — session PIN Mode Vente (N°66). Une entrée naît au login d'un
// revendeur dont la limite d'appareils est active (MaxDevices > 0) ; son
// identifiant (jti) est embarqué dans le JWT émis et recontrôlé à CHAQUE
// requête /api/sell/* : un token dont la session a été évincée (limite
// atteinte, limite baissée, revendeur supprimé) est refusé en 401. Les
// sessions plus vieilles que le TTL du token + 1 h de grâce sont purgées au
// login suivant du même revendeur (aucune éclosion silencieuse).

// SellSession — session PIN Mode Vente (N°66). Une entrée naît au login d'un
// revendeur dont la limite d'appareils est active (MaxDevices > 0) ; son
// identifiant (jti) est embarqué dans le JWT émis et recontrôlé à CHAQUE
// requête /api/sell/* : un token dont la session a été évincée (limite
// atteinte, limite baissée, revendeur supprimé) est refusé en 401. Les
// sessions plus vieilles que le TTL du token + 1 h de grâce sont purgées au
// login suivant du même revendeur (aucune éclosion silencieuse).
type SellSession struct {
	ID         string `json:"id"`        // jti du JWT (« dev-… »)
	AccountID  string `json:"accountId"` // isolation multi-tenant
	ResellerID string `json:"resellerId"`
	IssuedAt   string `json:"issuedAt"`  // RFC3339 — clé d'éviction (FIFO login)
	LastSeen   string `json:"lastSeen"`  // = issuedAt (réservé : LRU futur)
	UserAgent  string `json:"userAgent"` // étiquette appareil, bornée 200 car.
	IP         string `json:"ip"`
}

// Transaction — mouvement de portefeuille revendeur (credit | sale).

// Transaction — mouvement de portefeuille revendeur (credit | sale).
type Transaction struct {
	ID           string `json:"id"`
	AccountID    string `json:"accountId"`
	Type         string `json:"type"`
	ResellerID   string `json:"resellerId"`
	ResellerName string `json:"resellerName"`
	Amount       int    `json:"amount"`
	Note         string `json:"note"`
	At           string `json:"at"`
}

// Rôles d'équipe (N°7) — hiérarchie de privilèges croissante. Le rôle
// « admin » historique (= super-admin plateforme) devient RolePlatformAdmin ;
// « admin » reste accepté en lecture pour les tokens/JWT existants.

// Rôles d'équipe (N°7) — hiérarchie de privilèges croissante. Le rôle
// « admin » historique (= super-admin plateforme) devient RolePlatformAdmin ;
// « admin » reste accepté en lecture pour les tokens/JWT existants.
const (
	RoleManager       = "manager"        // gérant : tout le compte SAUF équipe et réglages/billing
	RoleOwner         = "owner"          // propriétaire du compte : tout, y compris équipe
	RolePlatformAdmin = "platform_admin" // super-admin MikCloud (multi-comptes)
)

// Usages de compte SaaS (N°98) — un compte EST hotspot OU homenet, il ne
// bascule pas (contrairement à la console plateforme qui se choisit par
// rôle). Le cas « je gère un cyber ET ma maison » = deux comptes (comme
// deux espaces Slack), pas un mode hybride.
const (
	AccountUsageHotspot = "hotspot" // réseaux publics payants — vouchers, revendeurs, Mode Vente
	AccountUsageHomeNet = "homenet" // réseaux privés domestiques — appareils, famille, couvre-feu
)

// Activity — journal d'activité/audit (N°7 : trace QUI a agi, pas seulement quoi).

// Activity — journal d'activité/audit (N°7 : trace QUI a agi, pas seulement quoi).
type Activity struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	Type      string `json:"type"` // router | user | voucher | reseller | session | system | team
	Message   string `json:"message"`
	At        string `json:"at"`
	// N°7 — acteur authentifié à l'origine de l'action (vide = moteur interne :
	// simulation, agent routeur, notifications automatiques).
	ActorID   string `json:"actorId,omitempty"`
	ActorName string `json:"actorName,omitempty"`
}

// Sale — vente de vouchers (par lot), attribuée au routeur (site) émetteur.

// Sale — vente de vouchers (par lot), attribuée au routeur (site) émetteur.
type Sale struct {
	ID           string `json:"id"`
	AccountID    string `json:"accountId"`
	Amount       int    `json:"amount"` // conserve sa sémantique : price × count
	ProfileName  string `json:"profileName"`
	Count        int    `json:"count"`
	Channel      string `json:"channel"` // direct | reseller
	ResellerName string `json:"resellerName"`
	RouterID     string `json:"routerId"`
	RouterName   string `json:"routerName"`
	BatchID      string `json:"batchId"`
	At           string `json:"at"`
	// P2 (audit Mikhmon) — marge (F13) : Cost = price×count,
	// SellingTotal = (sellingPrice || price)×count.
	Cost         int `json:"cost"`
	SellingTotal int `json:"selling"`
}

// Batch — lot de vouchers générés en une fois (traçabilité complète).

// Batch — lot de vouchers générés en une fois (traçabilité complète).
type Batch struct {
	ID          string `json:"id"`
	AccountID   string `json:"accountId"`
	ProfileID   string `json:"profileId"`
	ProfileName string `json:"profileName"`
	RouterID    string `json:"routerId"`
	RouterName  string `json:"routerName"`
	Count       int    `json:"count"`
	UnitPrice   int    `json:"unitPrice"`
	TotalCost   int    `json:"totalCost"`
	// DataQuotaMb — quota de données (Mo) porté par chaque voucher du lot
	// (0 = illimité). Tracé pour l'affichage et la comptabilité.
	DataQuotaMb int64 `json:"dataQuotaMb"`
	// Parité Mikhmon — Time Limit (limit-uptime) résolu à la génération du
	// lot (minutes ; 0 = héritage historique : profil sans quota de temps).
	TimeLimitMin int64  `json:"timeLimitMin"`
	Channel      string `json:"channel"` // direct | reseller
	ResellerID   string `json:"resellerId"`
	ResellerName string `json:"resellerName"`
	CreatedAt    string `json:"createdAt"`
}

// Tenant — infos du tenant.
