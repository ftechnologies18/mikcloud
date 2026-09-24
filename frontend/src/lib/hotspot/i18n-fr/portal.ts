// Fragment FR du domaine « portal » — clés préfixées "portal.".
// Extrait du monolithe i18n.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const frPortal: Record<string, string> = {

  // — N°35-d — portail captif (vue Portail) —
  // N°186 — purge des clés mortes : title/subtitle (vue autonome
  // pré-N°57-d) et les libellés des anciens dialogs textareas
  // (customizeTitle/customizeHint/resetOverride/identitySection,
  // bannerUrl/style/styleInherit/welcome/whatsapp*/ticker/services/
  // slides) — remplacés par l'éditeur unifié et ses clés propres.
  "portal.empty": "Aucun routeur agent en ligne",
  "portal.emptyHint":
    "Les routeurs en mode agent déploient le portail automatiquement. Ajoutez un routeur en mode agent dans la section Infrastructure pour activer le portail captif.",
  "portal.sigDeployed": "Portail à jour",
  "portal.sigPending": "Re-déploiement en attente",
  "portal.sigNever": "Jamais déployé",
  "portal.preview": "Aperçu",
  "portal.previewTitle": "Aperçu du portail — {name}",
  "portal.previewHint":
    "Aperçu statique du portail (le fetch live n'est pas actif en aperçu — le branding se rafraîchit automatiquement côté routeur).",
  "portal.redeploy": "Re-déployer",
  "portal.redeployConfirmTitle": "Re-déploiement du portail",
  "portal.redeployConfirm":
    "Forcer le re-déploiement du portail sur «{name}» ? Le routeur rechargera tous les fichiers (login.html, status.html, assets) à son prochain check-in (≤ 45 s).",
  "portal.redeployQueued": "Re-déploiement programmé",
  "portal.cancel": "Annuler",
  // N°135 — le logo du portail est celui du client : où le poser.
  // N°184 — l'onglet Expérience a disparu (fusion dans l'onglet Portail) :
  // le logo et le branding du compte se posent dans la section « Portail
  // du compte », en tête de l'onglet Portail.
  "portal.brandingNote":
    "Le logo affiché sur le portail est celui de votre établissement : posez-le dans la section « Portail du compte » en tête de l'onglet Portail — la bannière d'en-tête, la liste « Nos Services » et le bandeau animé sous le logo se règlent dans le même formulaire. Tout changement de branding est re-déployé automatiquement sur vos routeurs (≤ 45 s).",
  "portal.journal": "Journal des déploiements",
  "portal.journalEmpty": "Aucun déploiement enregistré",

  // — N°182 — sites physiques + personnalisation par site et par routeur —
  // N°186 — la note mentionne le sélecteur « Vous personnalisez » (éditeur
  // unifié compte / site / routeur en tête d'onglet).
  "portal.chainNote":
    "Chaque routeur sert le premier portail défini en remontant sa chaîne : sa personnalisation propre → celle de son site → celle du compte. Choisissez le niveau à éditer avec « Vous personnalisez » : chaque groupe non personnalisé hérite du niveau supérieur, et tout changement est re-déployé automatiquement (≤ 45 s).",
  "portal.sites": "Sites",
  "portal.sitesHint":
    "Regroupez vos routeurs par établissement (bâtiment, boutique, campus) et donnez à chaque site sa propre identité de portail.",
  "portal.newSite": "Nouveau site",
  "portal.noSites":
    "Aucun site — tous vos routeurs servent le portail du compte.",
  "portal.siteRouters": "{count} routeur(s)",
  "portal.sitePortalCustom": "Portail personnalisé",
  "portal.sitePortalAccount": "Portail du compte",
  "portal.siteEdit": "Modifier",
  "portal.siteDelete": "Supprimer",
  "portal.siteDeleteTitle": "Supprimer le site «{name}» ?",
  "portal.siteDeleteConfirm":
    "Ses {count} routeur(s) seront détachés et serviront à nouveau le portail du compte — mise à jour automatique au check-in suivant (≤ 45 s).",
  "portal.siteDeleted": "Site supprimé",
  "portal.siteCreated": "Site créé",
  "portal.siteCreatedDesc": "L'éditeur s'ouvre sur son portail : personnalisez-le groupe par groupe.",
  "portal.siteUpdated": "Site modifié",
  "portal.siteName": "Nom du site",
  "portal.siteLocation": "Localisation (ville, quartier)",
  "portal.siteDescription": "Description",
  "portal.siteCustomize": "Portail du site",
  "portal.siteDialogBrandingHint":
    "L'identité du portail du site (logo, bannière, services, messages…) se personnalise dans l'éditeur « Vous personnalisez » en tête d'onglet — le bouton « Portail du site » de sa carte y bascule.",
  "portal.routerSection": "Routeurs",
  "portal.routerSite": "Site",
  "portal.routerNoSite": "Hors site (portail du compte)",
  "portal.regimeAccount": "Portail : compte",
  "portal.regimeSite": "Portail : site",
  "portal.regimeCustom": "Portail : personnalisé",
  "portal.customize": "Personnaliser",
  "portal.overrideSaved": "Portail du routeur mis à jour — re-déploiement ≤ 45 s",
  "portal.siteSaved": "Site enregistré — portails re-déployés automatiquement (≤ 45 s)",
  "portal.assignSaved": "Routeur rattaché au site",
  "portal.displayName": "Nom affiché sur le portail",
  "portal.logoUrl": "Logo (data:image/… ≤ 300 Ko)",
  "portal.waveLink": "Lien marchand Wave",
  "portal.styleCommercial": "Commercial",
  "portal.styleHospitality": "Hospitalité",
  "portal.save": "Enregistrer",

  // — N°186 — éditeur UNIFIÉ à sélecteur de contexte (Compte / Site /
  // Routeur) — les mêmes briques riches aux trois niveaux, l'héritage
  // rendu visible (valeur résolue + provenance par groupe). —
  "portal.editorSection": "Éditeur du portail — chaîne Compte · Site · Routeur",
  "portal.ctxLabel": "Vous personnalisez :",
  "portal.ctxPlaceholder": "Compte, site ou routeur…",
  "portal.ctxGroupAccount": "Compte",
  "portal.ctxAccountItem": "Compte — {name}",
  "portal.editorHintAccount": "La base de la chaîne : les sites puis les routeurs héritent de ces réglages.",
  "portal.editorHintSite": "Les groupes non personnalisés héritent du compte — {n} routeur(s) servent ce portail.",
  "portal.editorHintRouter": "Les groupes non personnalisés héritent de {source}, puis du compte.",
  "portal.editorHintNone": "Sélectionnez le niveau à personnaliser : chaque groupe affiche d'abord la valeur héritée.",
  "portal.editorEmptyTitle": "Choisissez le portail à personnaliser",
  "portal.editorEmptyHint":
    "Sélectionnez un site ou un routeur dans le sélecteur ci-dessus, ou cliquez « Personnaliser » sur une carte ci-dessous. Chaque groupe affiche la valeur héritée du niveau supérieur avant personnalisation.",
  "portal.editorSiteTitle": "Portail du site «{name}»",
  "portal.editorRouterTitle": "Portail du routeur «{name}»",
  "portal.editorOverrideDesc":
    "Personnalisez groupe par groupe : un groupe « Hérité » suit le niveau supérieur, un groupe « Personnalisé » s'applique ici. Un seul bouton enregistre (Ctrl+Entrée fonctionne aussi).",
  "portal.editorOverrideCardDesc":
    "Chaque groupe laissé « Hérité » suit le niveau supérieur ; « Personnaliser » le pré-remplit avec la valeur héritée, « Réinitialiser » le lui rend. Enregistrer re-déploie le portail au check-in suivant (≤ 45 s).",
  "portal.editorSiteSaved": "Portail du site mis à jour — re-déploiement ≤ 45 s",
  "portal.navIdentity": "Identité",
  "portal.identityCard": "Identité du portail",
  "portal.identityCardDesc": "Nom affiché, logo et lien marchand Wave propres à ce niveau.",
  "portal.identityNamePlaceholder": "Nom affiché sur le portail",
  "portal.identityLogoHint":
    "Image ≤ 300 Ko, intégrée (data:image/…) — remplace le logo du compte pour ce niveau.",
  "portal.groupInherited": "Hérité",
  "portal.groupPersonalized": "Personnalisé",
  "portal.groupPersonalize": "Personnaliser",
  "portal.groupReset": "Réinitialiser (hériter)",
  "portal.inheritedFromAccount": "Hérité du compte",
  "portal.inheritedFromSite": "Hérité du site «{name}»",
  "portal.summaryNothing": "Rien de réglé au-dessus — le portail garde ses valeurs par défaut.",
  "portal.summarySlides": "{n} slide(s)",
  "portal.summaryPromos": "{n} promo(s)",
  "portal.summarySocials": "{n} réseau(x)",
  "portal.summaryWave": "Wave lié",
  "portal.removeOverride": "Retirer toute la personnalisation",
  "portal.removeOverrideTitle": "Retirer toute la personnalisation ?",
  "portal.removeOverrideDesc":
    "Tous les groupes personnalisés de ce niveau reviendront hériter du niveau supérieur — re-déploiement automatique au check-in suivant (≤ 45 s).",
  "portal.overrideRemoved": "Personnalisation retirée — le portail hérite à nouveau (≤ 45 s)",
  "portal.ctxGuardTitle": "Changer de portail ?",
  "portal.ctxGuardDesc":
    "{n} modification(s) non enregistrée(s) sur le portail en cours d'édition seront perdues.",
};
