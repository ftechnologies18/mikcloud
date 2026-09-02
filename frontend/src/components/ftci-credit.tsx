/**
 * FtciCredit — mention « © 2026 FTCI — Freelance Technologies Côte d'Ivoire »
 * cliquable vers le site d'atterrissage https://ftci.fr/ (nouvel onglet).
 * Présente sur les trois surfaces de l'app : landing (pied de page), écran de
 * connexion (panneau de marque + pied mobile) et console (sidebar desktop + menu mobile).
 */
export function FtciCredit({ className = "" }: { className?: string }) {
  return (
    <a
      href="https://ftci.fr/"
      target="_blank"
      rel="noopener noreferrer"
      aria-label="FTCI — Freelance Technologies Côte d'Ivoire, site officiel (s'ouvre dans un nouvel onglet)"
      className={`inline-flex items-center justify-center gap-1 transition-colors hover:text-foreground hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${className}`}
    >
      <span>© 2026 FTCI — Freelance Technologies Côte d'Ivoire</span>
    </a>
  );
}
