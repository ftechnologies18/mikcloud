import type { Metadata } from "next";
import Link from "next/link";
import Image from "next/image";
import {
  AlertTriangle,
  ArrowLeft,
  CalendarClock,
  Database,
  FileCheck2,
  HardDrive,
  Mail,
  Scale,
  Server,
  ShieldCheck,
  UserCog,
  Users,
} from "lucide-react";

import { Card, CardContent } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { FtciCredit } from "@/components/ftci-credit";

// N°70 — Page publique de la politique de confidentialité (docs/REGISTRE-
// TRAITEMENT.md §6.1 : « Publier la présente politique sur une page publique
// du frontend (/legal/confidentialite) et la lier depuis l'inscription »).
// Composant serveur : contenu statique, métadonnées pour le référencement et
// zéro JavaScript client — aligné avec la convention « pages visiteurs FR »
// (la version française fait foi, la loi de référence est ivoirienne).
// La source de vérité DU CONTENU est docs/REGISTRE-TRAITEMENT.md : toute
// évolution du registre doit être répercutée ici dans le même commit.

export const metadata: Metadata = {
  title: "Politique de confidentialité — MikCloud",
  description:
    "Traitement des données personnelles MikCloud (loi ivoirienne n° 2013-450, ARTCI/CDP) : finalités, durées de conservation, sous-traitants, mesures de sécurité et droits des personnes.",
  keywords: ["confidentialité", "données personnelles", "2013-450", "ARTCI", "CDP", "RGPD"],
};

/** Contact privacy du registre §6.2 (opérateur FTech CI). */
const PRIVACY_EMAIL = "privacy@mikcloud.ftci.fr";

function SectionTitle({
  icon: Icon,
  children,
}: {
  icon: typeof ShieldCheck;
  children: React.ReactNode;
}) {
  return (
    <h2 className="mt-10 flex items-center gap-2.5 text-lg font-semibold tracking-tight">
      <Icon className="size-5 shrink-0 text-primary" aria-hidden />
      {children}
    </h2>
  );
}

export default function LegalConfidentialityPage() {
  return (
    <div className="flex min-h-screen flex-col bg-background text-foreground">
      {/* ─── En-tête ─── */}
      <header className="border-b border-border/60 bg-card/30">
        <div className="mx-auto flex w-full max-w-3xl items-center justify-between gap-4 px-4 py-4 sm:px-6">
          <Link
            href="/"
            className="flex items-center gap-2.5 rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            aria-label="MikCloud — retour à l'accueil"
          >
            <Image
              src="/logo.png"
              alt="Logo MikCloud"
              width={36}
              height={36}
              className="rounded-lg shadow-md shadow-primary/20"
            />
            <span className="text-base font-bold">MikCloud</span>
          </Link>
          <Link
            href="/"
            className="inline-flex min-h-11 items-center gap-1.5 rounded-md px-2 text-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <ArrowLeft className="size-4" aria-hidden />
            Retour à l&rsquo;accueil
          </Link>
        </div>
      </header>

      {/* ─── Contenu ─── */}
      <main className="mx-auto w-full max-w-3xl flex-1 px-4 py-8 sm:px-6 sm:py-10">
        <article>
          <h1 className="text-2xl font-bold tracking-tight sm:text-3xl">
            Politique de confidentialité
          </h1>
          <p className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
            <span className="inline-flex items-center gap-1.5">
              <CalendarClock className="size-3.5" aria-hidden />
              Dernière mise à jour : 9 septembre 2026
            </span>
            <span className="inline-flex items-center gap-1.5">
              <Scale className="size-3.5" aria-hidden />
              Loi n° 2013-450 du 19 juin 2013 (Côte d&rsquo;Ivoire)
            </span>
          </p>

          <Card className="mt-6 border-primary/20 bg-primary/5">
            <CardContent className="space-y-3 p-5 text-sm leading-relaxed">
              <p>
                <strong className="font-semibold">MikCloud</strong> est un service de gestion de
                hotspots MikroTik exploité par{" "}
                <strong className="font-semibold">
                  FTCI — Freelance Technologies Côte d&rsquo;Ivoire
                </strong>{" "}
                (« l&rsquo;opérateur », « nous »), Abidjan, Côte d&rsquo;Ivoire.
              </p>
              <p>
                La présente politique décrit quelles données personnelles sont collectées, pour
                quelles finalités, pendant combien de temps elles sont conservées, qui y accède et
                comment exercer vos droits. Elle est établie au titre de la{" "}
                <strong className="font-semibold">loi ivoirienne n° 2013-450</strong> relative à la
                protection des données à caractère personnel (autorité ARTCI/CDP) ; pour les
                personnes situées hors de Côte d&rsquo;Ivoire, le RGPD (règlement UE 2016/679) est
                appliqué comme référence de bonnes pratiques.
              </p>
              <p className="flex flex-wrap items-center gap-1.5">
                <Mail className="size-4 shrink-0 text-primary" aria-hidden />
                Toute demande relative à vos données :{" "}
                <a
                  href={`mailto:${PRIVACY_EMAIL}`}
                  className="font-medium text-primary underline underline-offset-4 hover:opacity-80"
                >
                  {PRIVACY_EMAIL}
                </a>
              </p>
            </CardContent>
          </Card>

          {/* ─── 1. Traitements ─── */}
          <SectionTitle icon={Database}>Données traitées, finalités et durées</SectionTitle>
          <p className="mt-3 text-sm leading-relaxed text-muted-foreground">
            Le registre ci-dessous récapitule l&rsquo;ensemble des traitements mis en œuvre par la
            plateforme (source : registre des traitements de l&rsquo;opérateur).
          </p>
          <div className="mt-4 overflow-x-auto rounded-lg border border-border/60">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-32">Traitement</TableHead>
                  <TableHead>Finalité</TableHead>
                  <TableHead>Données</TableHead>
                  <TableHead className="w-36">Base légale</TableHead>
                  <TableHead className="w-44">Durée</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody className="text-sm">
                <TableRow>
                  <TableCell className="font-medium">T1 · Compte SaaS</TableCell>
                  <TableCell>Inscription, authentification, 2FA</TableCell>
                  <TableCell>
                    Identité (nom, identifiant), email, WhatsApp, pays/ville, mot de passe (bcrypt),
                    secret TOTP, sessions
                  </TableCell>
                  <TableCell>Contrat (art. 9)</TableCell>
                  <TableCell>Durée du compte + 12 mois</TableCell>
                </TableRow>
                <TableRow>
                  <TableCell className="font-medium">T2 · Gestion hotspot</TableCell>
                  <TableCell>Vouchers, utilisateurs hotspot, sessions, trafic</TableCell>
                  <TableCell>Identifiants hotspot, MAC/IP, volumes, horodatages</TableCell>
                  <TableCell>Contrat</TableCell>
                  <TableCell>
                    Durée du compte ; journal de connexion (login/IP/MAC) : 30/60/90 jours selon le
                    réglage du compte (défaut 90, purge automatique horaire)
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell className="font-medium">T3 · Journal &amp; sécurité</TableCell>
                  <TableCell>Traçabilité, détection de force brute</TableCell>
                  <TableCell>
                    Identifiants de connexion, IP (premier hop), user-agent, horodatages
                  </TableCell>
                  <TableCell>Intérêt légitime (sécurité)</TableCell>
                  <TableCell>12 mois maximum</TableCell>
                </TableRow>
                <TableRow>
                  <TableCell className="font-medium">T4 · Facturation</TableCell>
                  <TableCell>Essai, plans, paiements Wave / GeniusPay</TableCell>
                  <TableCell>
                    Transactions, statut d&rsquo;abonnement (aucune donnée carte stockée)
                  </TableCell>
                  <TableCell>Contrat + obligation légale</TableCell>
                  <TableCell>5 ans (comptable)</TableCell>
                </TableRow>
                <TableRow>
                  <TableCell className="font-medium">T5 · Notifications</TableCell>
                  <TableCell>Alertes opérationnelles (email / WhatsApp / Telegram)</TableCell>
                  <TableCell>Coordonnées du gérant</TableCell>
                  <TableCell>Contrat</TableCell>
                  <TableCell>Durée du compte</TableCell>
                </TableRow>
                <TableRow>
                  <TableCell className="font-medium">T6 · Marketing WiFi</TableCell>
                  <TableCell>
                    Envoi d&rsquo;actualités et d&rsquo;offres par le gestionnaire du site WiFi
                    (optionnel)
                  </TableCell>
                  <TableCell>
                    Numéro de téléphone, preuve horodatée du consentement (date et geste
                    d&rsquo;opt-in)
                  </TableCell>
                  <TableCell>Consentement (art. 9)</TableCell>
                  <TableCell>
                    Jusqu&rsquo;au retrait (« Ne plus recevoir », effet immédiat) ou à la
                    suppression du site
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </div>
          <p className="mt-3 text-xs leading-relaxed text-muted-foreground">
            L&rsquo;inscription marketing WiFi (T6) est{" "}
            <strong className="font-medium">
              désactivée par défaut : rien n&rsquo;est coché d&rsquo;avance, refuser ne retire
              aucun service
            </strong>{" "}
            (le code WiFi arrive pareil), et le retrait reste possible à tout moment en un geste sur
            la page du code WiFi.
          </p>

          {/* ─── 2. Sous-traitants ─── */}
          <SectionTitle icon={Server}>Sous-traitants et destinataires</SectionTitle>
          <div className="mt-4 overflow-x-auto rounded-lg border border-border/60">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-44">Prestataire</TableHead>
                  <TableHead>Rôle</TableHead>
                  <TableHead className="w-40">Localisation</TableHead>
                  <TableHead>Garanties</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody className="text-sm">
                <TableRow>
                  <TableCell className="font-medium">Neon (PostgreSQL managé)</TableCell>
                  <TableCell>Hébergement de la base de données</TableCell>
                  <TableCell>UE (eu-central-1)</TableCell>
                  <TableCell>Chiffrement TLS, clés managées, contrat de sous-traitance</TableCell>
                </TableRow>
                <TableRow>
                  <TableCell className="font-medium">Render</TableCell>
                  <TableCell>Exécution du backend</TableCell>
                  <TableCell>UE / US</TableCell>
                  <TableCell>SOC 2, TLS</TableCell>
                </TableRow>
                <TableRow>
                  <TableCell className="font-medium">Vercel</TableCell>
                  <TableCell>Hébergement du frontend</TableCell>
                  <TableCell>Edge mondial</TableCell>
                  <TableCell>SOC 2</TableCell>
                </TableRow>
                <TableRow>
                  <TableCell className="font-medium">Wave, GeniusPay</TableCell>
                  <TableCell>Passerelles de paiement</TableCell>
                  <TableCell>—</TableCell>
                  <TableCell>Aucune donnée carte ne transite par MikCloud</TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </div>
          <p className="mt-3 text-sm leading-relaxed text-muted-foreground">
            Aucune revente ni partage publicitaire des données :{" "}
            <strong className="font-medium">pas de profilage</strong>, pas de cookies publicitaires.
          </p>

          {/* ─── 3. Stockage local ─── */}
          <SectionTitle icon={HardDrive}>Stockage local sur votre appareil</SectionTitle>
          <ul className="mt-3 space-y-2 text-sm leading-relaxed text-muted-foreground">
            <li className="flex gap-2">
              <span aria-hidden>•</span>
              <span>
                <strong className="font-medium text-foreground">Session de travail</strong> :
                identifiant de session, langue et préférences d&rsquo;interface (localStorage
                « mikcloud-auth »), purgés à la déconnexion.
              </span>
            </li>
            <li className="flex gap-2">
              <span aria-hidden>•</span>
              <span>
                <strong className="font-medium text-foreground">Mode Vente hors ligne</strong> :
                file locale de ventes en attente de synchronisation (IndexedDB), purgée dès que la
                connexion revient.
              </span>
            </li>
            <li className="flex gap-2">
              <span aria-hidden>•</span>
              <span>
                Aucun traceur tiers, aucun cookie publicitaire, aucune mesure d&rsquo;audience
                individuelle.
              </span>
            </li>
          </ul>

          {/* ─── 4. Sécurité ─── */}
          <SectionTitle icon={ShieldCheck}>Sécurité</SectionTitle>
          <ul className="mt-3 space-y-2 text-sm leading-relaxed text-muted-foreground">
            <li className="flex gap-2">
              <span aria-hidden>•</span>
              Mots de passe protégés par bcrypt (coût 12), politique de robustesse (10 caractères
              minimum, liste d&rsquo;interdits) ;
            </li>
            <li className="flex gap-2">
              <span aria-hidden>•</span>
              Sessions signées (JWT 24 h) révocables immédiatement ; double authentification (2FA
              TOTP) disponible ;
            </li>
            <li className="flex gap-2">
              <span aria-hidden>•</span>
              Chiffrement du transport TLS partout (HSTS), restrictions d&rsquo;accès strictes
              (CORS en échec fermé) ;
            </li>
            <li className="flex gap-2">
              <span aria-hidden>•</span>
              Sauvegardes chiffrées (AES-256-GCM) avec test de restauration automatisé chaque
              semaine ;
            </li>
            <li className="flex gap-2">
              <span aria-hidden>•</span>
              Journalisation des échecs d&rsquo;authentification et limitation de débit par IP
              (protection force brute) ;
            </li>
            <li className="flex gap-2">
              <span aria-hidden>•</span>
              Chaîne d&rsquo;approvisionnement surveillée en continu (audit de vulnérabilités,
              dépendances, détection de secrets).
            </li>
          </ul>

          {/* ─── 5. Droits ─── */}
          <SectionTitle icon={Users}>Vos droits</SectionTitle>
          <div className="mt-4 overflow-x-auto rounded-lg border border-border/60">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-40">Droit</TableHead>
                  <TableHead>Mise en œuvre par MikCloud</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody className="text-sm">
                <TableRow>
                  <TableCell className="font-medium">Information</TableCell>
                  <TableCell>
                    Présente page + mentions à l&rsquo;inscription (case « politique de
                    confidentialité »).
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell className="font-medium">Accès / portabilité</TableCell>
                  <TableCell>
                    Exports CSV intégrés par module (vouchers, ventes, utilisateurs hotspot) ; export
                    complet du compte sur demande support, restitué chiffré.
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell className="font-medium">Rectification</TableCell>
                  <TableCell>
                    Paramètres → Général (organisation), identifiants et équipe par le gérant ;
                    support pour l&rsquo;email.
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell className="font-medium">Suppression</TableCell>
                  <TableCell>
                    Fermeture du compte sur demande : anonymisation des données personnelles,
                    détachement des données techniques ; les enregistrements comptables sont
                    conservés en forme anonymisée pendant la durée légale (5 ans). Délai : 30 jours
                    maximum.
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell className="font-medium">Opposition / limitation</TableCell>
                  <TableCell>
                    Notifications désactivables à tout moment (Paramètres → Notifications) ;
                    retrait du marketing WiFi en un geste (« Ne plus recevoir »).
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell className="font-medium">Réclamation</TableCell>
                  <TableCell>
                    Auprès de la CDP / ARTCI (Côte d&rsquo;Ivoire) — procédure communiquée sur
                    demande.
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </div>

          {/* ─── 6. Violations de données ─── */}
          <SectionTitle icon={AlertTriangle}>Violations de données</SectionTitle>
          <p className="mt-3 text-sm leading-relaxed text-muted-foreground">
            En cas de violation de données personnelles : détection par les journaux de sécurité →
            qualification sous 48 heures →{" "}
            <strong className="font-medium text-foreground">
              notification de l&rsquo;ARTCI / CDP sous 72 heures
            </strong>{" "}
            si un risque est identifié → information des personnes concernées si le risque est élevé
            → consignation de l&rsquo;incident dans le journal dédié.
          </p>

          {/* ─── 7. Évolution ─── */}
          <SectionTitle icon={FileCheck2}>Évolution de cette politique</SectionTitle>
          <p className="mt-3 text-sm leading-relaxed text-muted-foreground">
            Cette politique peut évoluer avec le service : la date de mise à jour figure en tête de
            page et toute modification substantielle est annoncée dans la console (notifications).
            La version française fait foi. Pour toute question :{" "}
            <a
              href={`mailto:${PRIVACY_EMAIL}`}
              className="font-medium text-primary underline underline-offset-4 hover:opacity-80"
            >
              {PRIVACY_EMAIL}
            </a>
            .
          </p>
        </article>
      </main>

      {/* ─── Pied de page ─── */}
      <footer className="mt-auto border-t border-border/60 bg-card/30">
        <div className="mx-auto flex w-full max-w-3xl flex-col items-center gap-3 px-4 py-5 text-center sm:flex-row sm:justify-between sm:px-6 sm:text-left">
          <FtciCredit className="text-xs text-muted-foreground" />
          <p className="text-xs text-muted-foreground">
            <UserCog className="mr-1 inline size-3.5" aria-hidden />
            Responsable du traitement : FTCI — Freelance Technologies Côte d&rsquo;Ivoire
          </p>
        </div>
      </footer>
    </div>
  );
}
