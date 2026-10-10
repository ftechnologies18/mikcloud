// Fragment FR du domaine « vpn » — WireGuard vendable (N°291).
// Clés préfixées "vpn." ; miroir exact de i18n-en/vpn.ts.

export const frVpn: Record<string, string> = {
  "vpn.title": "VPN WireGuard",
  "vpn.description":
    "Vendez des accès VPN WireGuard à vos clients — confs générées sur votre serveur, livrées par QR, e-mail ou Telegram, révocables d'un clic.",

  // — Activation du module (opt-in, même doctrine D6 que le Cybercafé) —
  "vpn.activate.title": "Module VPN désactivé",
  "vpn.activate.desc":
    "Activez le module pour créer des accès VPN WireGuard pour vos clients. Chaque accès sort par votre serveur MikCloud (IP fixe), la vente peut être enregistrée à la caisse, et la révocation coupe le client à chaud — sans toucher aux autres accès.",
  "vpn.activate.cta": "Activer le module",
  "vpn.activate.toast": "Module VPN activé",
  "vpn.deactivate.toast": "Module VPN désactivé (les accès actifs restent en service)",
  "vpn.settingsHint":
    "Module actif — le bouton ci-dessous le désactive. Aucun accès actif n'est coupé : la révocation reste un geste par peer.",
  "vpn.deactivate.cta": "Désactiver le module",

  // — KPIs + santé VM —
  "vpn.kpi.peers": "Accès actifs",
  "vpn.kpi.peersSub": "peers VPN en service",
  "vpn.kpi.slots": "Slots wg0",
  "vpn.kpi.slotsSub": "{used} / {pool} adresses réservées (routeurs inclus)",
  "vpn.kpi.vm": "Serveur VPN",
  "vpn.kpi.vmUp": "VM joignable",
  "vpn.kpi.vmUpSub": "wg-mini répond sur la VM",
  "vpn.kpi.vmDown": "VM injoignable",
  "vpn.kpi.vmDownSub": "vérifiez le service wg-mini (systemctl status wg-mini)",
  "vpn.kpi.caisse": "Caisse du jour",
  "vpn.kpi.caisseSub": "ventes encaissées (tous canaux)",

  // — Table —
  "vpn.search": "Rechercher un accès…",
  "vpn.access": "Accès",
  "vpn.name": "Nom VM",
  "vpn.address": "Adresse tunnel",
  "vpn.state": "Statut",
  "vpn.created": "Créé le",
  "vpn.actions": "Actions",
  "vpn.state.active": "Actif",
  "vpn.state.pending": "Création…",
  "vpn.state.error": "Erreur",
  "vpn.kind.fulltunnel": "Client final",
  "vpn.kind.remote": "Accès distant",
  "vpn.conf": "Configuration",
  "vpn.deliver": "Livrer",
  "vpn.revoke": "Révoquer",
  "vpn.more": "Plus d'actions",
  "vpn.noAddress": "—",

  // — États vides —
  "vpn.empty.title": "Aucun accès VPN vendu",
  "vpn.empty.desc":
    "Créez un accès pour un client : la configuration est générée sur votre serveur et livrée en QR, e-mail ou Telegram.",
  "vpn.noMatch": "Aucun accès ne correspond",
  "vpn.noMatchDesc": "Essayez un autre nom ou libellé.",

  // — Création —
  "vpn.add": "Vendre un accès",
  "vpn.addTitle": "Vendre un accès VPN",
  "vpn.addDesc":
    "Les clés et la configuration sont générées sur votre serveur (la clé privée du client n'existe nulle part ailleurs). La génération prend quelques secondes.",
  "vpn.addLabel": "Libellé (nom du client, appareil…)",
  "vpn.addLabelPlaceholder": "Téléphone client Ali, PC boutique…",
  "vpn.addKind": "Forme d'accès",
  "vpn.addKind.fulltunnel": "Client final — tout le trafic sort par la VM (IP fixe)",
  "vpn.addKind.remote": "Accès distant du gérant — phase B (bientôt)",
  "vpn.addPrice": "Prix de vente enregistré (0 = ne rien encaisser)",
  "vpn.addPricePlaceholder": "Ex. 2000",
  "vpn.addPriceHint": "Saisi > 0 : la vente part à la caisse (canal direct) et dans les rapports.",
  "vpn.addWarning":
    "Le trafic du client sort par VOTRE serveur : gardez un œil sur la consommation (sortie ~10 To/mois sur l'offre Always Free) et révoquez les accès dormants.",
  "vpn.addCta": "Générer et vendre",
  "vpn.addToast": "Accès VPN créé",
  "vpn.addPending": "Génération en cours sur la VM…",

  // — Conf / livraison (D5-a) —
  "vpn.confTitle": "Configuration VPN",
  "vpn.confDesc":
    "Le client installe l'application WireGuard, puis « + » → importer. Gardez cette configuration secrète : elle donne un accès complet.",
  "vpn.confCopy": "Copier la configuration",
  "vpn.confCopied": "Configuration copiée",
  "vpn.confQr": "QR (scan direct)",
  "vpn.confEmail": "Envoyer par e-mail",
  "vpn.confTelegram": "Envoyer sur Telegram",
  "vpn.emailTitle": "Envoyer la configuration par e-mail",
  "vpn.emailDesc": "La configuration complète est envoyée au client (fournisseur e-mail de votre compte).",
  "vpn.emailTo": "Adresse e-mail du client",
  "vpn.emailCta": "Envoyer",
  "vpn.emailToast": "Envoi e-mail en cours",
  "vpn.telegramToast": "Conf envoyée sur Telegram",
  "vpn.revokeConfirmTitle": "Révoquer cet accès ?",
  "vpn.revokeConfirmDesc":
    "Le client perd immédiatement l'accès (serveur mis à jour à chaud). Les autres accès ne sont pas touchés. Cette action est définitive.",
  "vpn.revokeToast": "Accès révoqué",

  // — Réconciliation (check D4) —
  "vpn.reconcile": "Vérifier la VM",
  "vpn.reconcileOk": "VM vérifiée : {mine} accès(s) présent(s) sur le serveur",
  "vpn.reconcileMissing": "{n} accès(s) introuvable(s) sur la VM : {names}",
  "vpn.reconcileDown": "VM injoignable — réconciliation impossible",

  // — Divers —
  "vpn.errorToast": "Opération impossible",
  "vpn.error.miniUnconfigured":
    "Mini-service wg-mini non installé sur la VM — cf. deploy/oracle/wg-mini/README.md (l'exploitant l'installe en 10 minutes).",
  "vpn.disableHint":
    "La désactivation ne coupe PAS les accès actifs : le serveur continue de router les peers existants.",
};
