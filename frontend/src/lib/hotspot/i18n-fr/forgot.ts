// Fragment FR du domaine « forgot » — clés préfixées "forgot.".
// Extrait du monolithe i18n.ts (N°87) ; l'ordre des clés suit le fichier d'origine.
// Ne pas importer directement : passer par l'agrégateur (fusion + logique de résolution).

export const frForgot: Record<string, string> = {
  "forgot.title": "Réinitialiser votre mot de passe",
  "forgot.desc": "Saisissez l'e-mail enregistré à la création de votre compte : nous vous enverrons un lien de réinitialisation.",
  "forgot.email": "Adresse e-mail",
  "forgot.emailPlaceholder": "vous@exemple.ci",
  "forgot.emailInvalid": "Adresse e-mail invalide",
  "forgot.submit": "Envoyer le lien",
  "forgot.sending": "Envoi…",
  "forgot.sentTitle": "Lien envoyé",
  "forgot.sentDesc": "Un lien de réinitialisation a été envoyé à {email}. Il est valable {minutes} minutes et ne peut être utilisé qu'une seule fois.",
  "forgot.sentSpamHint": "Pensez à vérifier vos indésirables (spam) si l'e-mail n'arrive pas.",
  "forgot.sendFailed": "E-mail non envoyé — réessayez plus tard",
  "forgot.close": "Fermer",
};
