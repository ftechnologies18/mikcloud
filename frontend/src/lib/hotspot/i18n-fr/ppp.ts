// Fragment FR du domaine « ppp » — abonnés PPPoE (console WISP, N°294).
// Clés préfixées "ppp." ; miroir exact de i18n-en/ppp.ts.

export const frPpp: Record<string, string> = {
  "ppp.title": "Abonnés PPPoE",
  "ppp.description":
    "Gérez les abonnés du serveur PPPoE de vos routeurs : création, suspension, renouvellement et déconnexion — pilotés par l'agent MikCloud (aucun port public ouvert).",

  // — Sélection du routeur (toute la vue est scopée à UN routeur) —
  "ppp.routerLabel": "Routeur",
  "ppp.routerHint": "Le PPPoE est piloté par l'agent MikCloud : les routeurs en mode réel ne sont pas éligibles.",
  "ppp.noRouter.title": "Aucun routeur en mode agent",
  "ppp.noRouter.desc":
    "Les abonnés PPPoE vivent sur vos routeurs MikroTik. Ajoutez un routeur et installez l'agent MikCloud (mode agent) pour le piloter depuis cette console.",
  "ppp.noRouter.cta": "Aller aux routeurs",

  // — KPIs —
  "ppp.kpi.subscribers": "Abonnés",
  "ppp.kpi.subscribersSub": "registre du routeur sélectionné",
  "ppp.kpi.online": "En ligne",
  "ppp.kpi.onlineSub": "sessions actives (cache agent)",
  "ppp.kpi.onlineUnknown": "lecture en file…",
  "ppp.kpi.suspended": "Suspendus",
  "ppp.kpi.suspendedSub": "auto : {n}",
  "ppp.kpi.expiring": "Expirent ≤ 7 j",
  "ppp.kpi.expiringSub": "à renouveler bientôt",

  // — Onglets —
  "ppp.tab.subscribers": "Abonnés",
  "ppp.tab.sessions": "Sessions actives",
  "ppp.tab.discover": "Découverte",

  // — Table abonnés —
  "ppp.search": "Rechercher un abonné…",
  "ppp.name": "Nom",
  "ppp.profile": "Profil",
  "ppp.staticIp": "IP statique",
  "ppp.pool": "pool",
  "ppp.expires": "Échéance",
  "ppp.expiresNever": "Illimité",
  "ppp.expired": "expiré",
  "ppp.expiresIn": "J-{n}",
  "ppp.status": "Statut",
  "ppp.state.active": "Actif",
  "ppp.state.pending": "En attente",
  "ppp.state.error": "Erreur",
  "ppp.state.disabled": "Suspendu",
  "ppp.state.disabledAuto": "Suspendu (auto)",
  "ppp.parity": "Parité",
  "ppp.lastSeen": "vu {time}",
  "ppp.neverSeen": "jamais vu",
  "ppp.actions": "Actions",
  "ppp.more": "Plus d'actions",
  "ppp.selectAll": "Tout sélectionner",
  "ppp.errorRetry": "Réessayer",

  // — Sélection multiple —
  "ppp.bulk.selected": "{n} sélectionné(s)",
  "ppp.bulk.renew": "Renouveler la sélection",
  "ppp.bulk.renewDone": "{n} abonné(s) renouvelé(s)",
  "ppp.bulk.renewPartial": "{n} renouvelé(s), {m} en échec",

  // — États vides —
  "ppp.empty.title": "Aucun abonné sur ce routeur",
  "ppp.empty.desc":
    "Créez le premier abonné : le secret est posé sur le routeur au prochain check-in de l'agent (≤ 45 s). Le serveur PPPoE doit déjà exister côté routeur — sinon utilisez le provisionnement assisté.",
  "ppp.noMatch": "Aucun abonné ne correspond",
  "ppp.noMatchDesc": "Essayez un autre nom, profil ou commentaire.",

  // — Création —
  "ppp.add": "Nouvel abonné",
  "ppp.addTitle": "Créer un abonné PPPoE",
  "ppp.addDesc":
    "Le secret est enregistré au cloud puis posé sur le routeur par l'agent (commande en file — confirmation au prochain check-in, ≤ 45 s).",
  "ppp.addName": "Nom de l'abonné (identifiant de connexion)",
  "ppp.addNamePlaceholder": "ex. abonne1, jean.dupont@fai…",
  "ppp.addPassword": "Mot de passe PPP",
  "ppp.addPasswordPlaceholder": "secret de connexion",
  "ppp.addProfile": "Profil PPP (existant côté routeur)",
  "ppp.addProfilePlaceholder": "ex. mikcloud-ppp",
  "ppp.addProfileHint": "Le cloud ne crée pas les profils PPP : le nom doit déjà exister sur le routeur (ou venir du provisionnement assisté).",
  "ppp.addComment": "Commentaire (optionnel)",
  "ppp.addExpires": "Échéance (optionnelle)",
  "ppp.addExpiresHint": "Vide = illimité. Passée l'échéance : suspension automatique, rappel ou récurrent selon les réglages de l'abonné.",
  "ppp.addStatic": "IP statique (optionnelle)",
  "ppp.addStaticHint": "Vide = adresse attribuée par le pool du profil. L'IP doit être libre sur ce routeur.",
  "ppp.addCta": "Créer l'abonné",
  "ppp.addPending": "Création en cours…",
  "ppp.addToast": "Abonné créé (commande en file vers le routeur)",

  // — Renouvellement (F4) —
  "ppp.renew": "Renouveler",
  "ppp.renewTitle": "Renouveler {name}",
  "ppp.renewCurrent": "Échéance actuelle : {date}.",
  "ppp.renewNoDate": "Abonné illimité.",
  "ppp.renewAuto": "La nouvelle échéance part de la plus lointaine (actuelle ou aujourd'hui).",
  "ppp.renewDays": "Durée du renouvellement (jours)",
  "ppp.renewDaysHint": "1 à 3650 jours — le rappel d'échéance est ré-armé pour la nouvelle date.",
  "ppp.renewAutoNote": "Réactive un abonné suspendu automatiquement.",
  "ppp.renewSubmit": "Renouveler",
  "ppp.renewToast": "Abonné renouvelé",

  // — Modification —
  "ppp.edit": "Modifier",
  "ppp.editTitle": "Modifier {name}",
  "ppp.editDesc": "Seuls les champs modifiés sont envoyés au routeur (set partiel — confirmation au prochain check-in).",
  "ppp.editPassword": "Mot de passe PPP",
  "ppp.editPasswordHint": "Laisser vide pour ne pas changer le mot de passe.",
  "ppp.editProfile": "Profil PPP",
  "ppp.editComment": "Commentaire",
  "ppp.editExpires": "Échéance",
  "ppp.editExpiresHint": "Vide = illimité.",
  "ppp.editStatic": "IP statique",
  "ppp.editStaticHint": "Vide = pool du profil.",
  "ppp.editExpMode": "À l'échéance",
  "ppp.editExpMode.disable": "Suspendre l'abonné (défaut)",
  "ppp.editExpMode.none": "Ne rien faire (parité routeur seule)",
  "ppp.editAutoRenew": "Renouvellement automatique (récurrent, sans encaissement)",
  "ppp.editRenewDays": "Jours ajoutés à chaque échéance",
  "ppp.editRemind": "Rappel d'échéance",
  "ppp.editRemind.off": "Off",
  "ppp.editRemind.days": "{n} j avant",
  "ppp.editSubmit": "Enregistrer",
  "ppp.editToast": "Abonné modifié",

  // — Suspension / reprise —
  "ppp.suspend": "Suspendre",
  "ppp.resume": "Reprendre",
  "ppp.suspendConfirmTitle": "Suspendre cet abonné ?",
  "ppp.suspendConfirmDesc":
    "Le secret est désactivé sur le routeur (session coupée, nouvelles connexions refusées). Reprenez à tout moment — l'abonné et son échéance sont conservés.",
  "ppp.suspendToast": "Abonné suspendu",
  "ppp.resumeToast": "Abonné réactivé",

  // — Déconnexion —
  "ppp.kick": "Déconnecter (agent)",
  "ppp.kickLive": "Déconnecter (temps réel)",
  "ppp.kickToast": "Déconnexion en file vers le routeur",
  "ppp.kickLiveToast": "Session déconnectée (temps réel)",

  // — Suppression —
  "ppp.delete": "Supprimer",
  "ppp.deleteConfirmTitle": "Supprimer cet abonné ?",
  "ppp.deleteConfirmDesc":
    "Le secret est retiré du routeur à la confirmation de l'agent (≤ 45 s). La ligne disparaît du registre SEULEMENT après confirmation — jamais avant.",
  "ppp.deleteToast": "Suppression demandée",

  // — Onglet Sessions actives —
  "ppp.sessions.empty": "Aucune session PPPoE active",
  "ppp.sessions.emptyDesc": "Le cache se remplit au prochain rapport de l'agent (TTL 2 min) — ou lancez une lecture temps réel.",
  "ppp.sessions.queued": "Lecture en file — le rapport arrive au prochain check-in de l'agent…",
  "ppp.sessions.updated": "Cache agent : {time}",
  "ppp.sessions.col.name": "Abonné",
  "ppp.sessions.col.service": "Service",
  "ppp.sessions.col.address": "Adresse IP",
  "ppp.sessions.col.caller": "Caller ID",
  "ppp.sessions.col.uptime": "Uptime",
  "ppp.sessions.live": "Temps réel via tunnel",
  "ppp.sessions.livePending": "Lecture directe en cours…",
  "ppp.sessions.liveCount": "{n} session(s) · {ms} ms via le tunnel",
  "ppp.sessions.liveErrorTitle": "Lecture temps réel impossible",

  // — Onglet Découverte —
  "ppp.discover.title": "Secrets vus par le routeur",
  "ppp.discover.empty": "Aucun secret découvert",
  "ppp.discover.emptyDesc": "Le routeur n'a pas encore rapporté son pppoe-server (ou il est vide) — la lecture part automatiquement.",
  "ppp.discover.queued": "Lecture en file — rapport en cours…",
  "ppp.discover.updated": "Cache agent : {time}",
  "ppp.discover.note":
    "MikCloud ne modifie pas les secrets découverts : ils ont été créés hors MikCloud (Winbox, Mikhmon…). Créez un abonné dans l'onglet Abonnés seulement s'il n'existe pas déjà côté routeur.",
  "ppp.discover.col.name": "Nom",
  "ppp.discover.col.profile": "Profil",
  "ppp.discover.col.disabled": "Suspendu",
  "ppp.discover.col.service": "Service",
  "ppp.discover.col.comment": "Commentaire (routeur)",

  // — Renfort temps réel (phase B) —
  "ppp.renfort.title": "Temps réel via tunnel",
  "ppp.renfort.desc": "Lecture et déconnexion instantanées à travers le tunnel WireGuard — le canal agent (≤ 45 s) reste le socle.",
  "ppp.renfort.credsOk": "Credentials API posées",
  "ppp.renfort.credsMissing": "Credentials API absentes",
  "ppp.renfort.tunnelOk": "Tunnel WireGuard actif",
  "ppp.renfort.tunnelMissing": "Aucun tunnel (renfort WireGuard requis)",
  "ppp.renfort.configure": "Configurer",
  "ppp.renfort.remove": "Retirer les credentials",
  "ppp.renfort.removeConfirmTitle": "Retirer les credentials API ?",
  "ppp.renfort.removeConfirmDesc":
    "Le temps réel via tunnel redevient indisponible. Le pilotage par l'agent (≤ 45 s) n'est jamais affecté.",
  "ppp.renfort.removeToast": "Credentials retirées",
  "ppp.renfort.dialogTitle": "Credentials API RouterOS",
  "ppp.renfort.dialogDesc":
    "Réutilise le compte API du routeur : les credentials sont chiffrées au repos et ne sortent jamais du backend. Configurez le service API sur le routeur (/ip service enable api).",
  "ppp.renfort.username": "Nom d'utilisateur API",
  "ppp.renfort.usernamePlaceholder": "ex. mikcloud-api",
  "ppp.renfort.password": "Mot de passe API",
  "ppp.renfort.passwordPlaceholder": "mot de passe du compte API",
  "ppp.renfort.save": "Enregistrer les credentials",
  "ppp.renfort.saveToast": "Credentials enregistrées (chiffrées au repos)",
  "ppp.renfort.note":
    "Prérequis : API RouterOS activée sur le routeur (/ip service enable api) et routeur tunnelé (renfort WireGuard) — le backend dial 10.8.0.N:8728 à travers wg0.",

  // — Provisionnement assisté (phase B) —
  "ppp.provision": "Provisionner le serveur PPPoE",
  "ppp.provisionTitle": "Provisionnement assisté du serveur PPPoE",
  "ppp.provisionDesc":
    "Génère un script .rsc IDEMPOTENT (chaque bloc ne crée son objet que s'il est absent) : pool, profil et serveur PPPoE. Aucun secret, aucune commande filée — vous collez le script vous-même.",
  "ppp.provisionInterface": "Interface du LAN à bridge",
  "ppp.provisionInterfacePlaceholder": "ex. ether2",
  "ppp.provisionService": "Service-name",
  "ppp.provisionProfile": "Profil par défaut",
  "ppp.provisionPoolStart": "Début du pool d'adresses",
  "ppp.provisionPoolEnd": "Fin du pool d'adresses",
  "ppp.provisionLocal": "Adresse locale (optionnelle)",
  "ppp.provisionDns": "Serveurs DNS (optionnel — séparés par des virgules)",
  "ppp.provisionDnsPlaceholder": "ex. 1.1.1.1, 8.8.8.8",
  "ppp.provisionCta": "Générer le script",
  "ppp.provisionPending": "Génération…",
  "ppp.provisionCopy": "Copier le script",
  "ppp.provisionCopied": "Script copié",
  "ppp.provisionDownload": "Télécharger (.rsc)",
  "ppp.provisionScriptNote": "À coller dans le terminal du routeur — idempotent : relancer ne duplique rien.",
  "ppp.provisionWarnings": "Avertissements du serveur",

  // — Divers —
  "ppp.errorToast": "Opération impossible",
};
