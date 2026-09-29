// Fragment FR du domaine « shell » — clés préfixées "shell.".
// Extrait du monolithe i18n.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const frShell: Record<string, string> = {

  // — shell —
  "shell.profileMenu": "Menu du profil",
  "shell.userMenu": "Menu utilisateur",
  "shell.profile": "Profil",
  "shell.settings": "Paramètres",
  "shell.logout": "Se déconnecter",
  "shell.refreshed": "Actualisé",
  "shell.logoAlt": "Logo MikCloud",
  "shell.goPlatform": "Console plateforme",
  "shell.goClient": "Console client",
  "shell.pickAccountTitle": "Ouvrir la console d'un client",
  "shell.pickAccountHint": "Session support — vous agissez dans le compte choisi, à tout moment.",
  "shell.pickAccountEmpty": "Aucun compte client pour le moment",
  "shell.impersonatingBadge": "Session support",
  "shell.impersonatingAs": "Console de {name}",
  "shell.exitImpersonation": "Retour à la console plateforme",
  "shell.impersonateToast": "Console de {name} ouverte — session support",
  "shell.exitImpersonationToast": "Retour sur la console plateforme",
  "shell.impersonateError": "Ouverture de la console impossible",

  // N°195 — contrôle de la barre latérale (3 modes) : Étendu / Réduit /
  // Survol. Bascule rapide par le bouton du rail ou Ctrl+B.
  "shell.sidebarMode": "Barre latérale",
  "shell.sidebarModeExpanded": "Étendu",
  "shell.sidebarModeReduced": "Réduit",
  "shell.sidebarModeHover": "Survol",
  "shell.sidebarModeHoverHint": "Le survol du rail ouvre la barre latérale",
  "shell.sidebarCollapse": "Replier la barre latérale (Ctrl+B)",
  "shell.sidebarExpand": "Ouvrir la barre latérale (Ctrl+B)",
};
