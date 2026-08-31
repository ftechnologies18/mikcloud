// Landing page MikCloud — copie marketing bilingue FR/EN.
//
// Auto-contenue (à part du dictionnaire app i18n.ts) car la landing a un
// volume de copie marketing important (~150 clés) qui n'a pas vocation à
// vivre dans le dictionnaire applicatif. La langue courante est lue via
// le store zustand (useHotspotStore.lang) — pas de rechargement au switch.
//
// Inspiration (fusion des meilleurs patterns) :
//   - Spotipo  : hero axé revenu, 2 CTA, trust bar, feature grid avec CTA
//                inline, FAQ, final CTA sans carte bancaire
//   - Tanaza   : multi-segments (ISP/MSP/Enterprise), framing hardware,
//                multi-langue, pricing 2-tier "switch anytime"
//   - Cloudi-Fi: benefits grid 4 cartes (cloud-native, une console,
//                déploiement facile, conformité/souveraineté), showcase
//                méthodes d'onboarding, angle infrastructure-agnostic
//
// Positionnement : marché africain pan-continental (UEMOA + CEMAC + Afrique
// de l'Est + Nigeria + Ghana). Multi-opérateur (Orange, MTN, Moov, Safaricom,
// Airtel, Vodafone GH, 9Mobile, Glo), multi mobile-money (Wave, Orange Money,
// MTN MoMo, Moov, MPesa, Airtel Money), multi-devises (FCFA, NGN, GHS, KES...).

export type Lang = "fr" | "en";

export interface LandingCopy {
  header: {
    brand: string;
    nav: { features: string; benefits: string; pricing: string; faq: string };
    signIn: string;
    signUp: string;
    langLabel: string;
  };
  hero: {
    badge: string;
    title1: string;
    titleAccent: string;
    title2: string;
    subtitle: string;
    ctaPrimary: string;
    ctaSecondary: string;
    ctaSignUp: string;
    ctaDemoHint: string;
    trialHint: string;
    statBar: { value: string; label: string }[];
  };
  trust: {
    items: { icon: string; label: string }[];
  };
  // — Section "Benefits" (inspirée Cloudi-Fi) — 4 cartes axées bénéfices
  benefits: {
    eyebrow: string;
    title: string;
    subtitle: string;
    items: { icon: string; title: string; description: string }[];
  };
  features: {
    eyebrow: string;
    title: string;
    subtitle: string;
    items: {
      icon: string;
      title: string;
      description: string;
      cta: string;
    }[];
  };
  how: {
    eyebrow: string;
    title: string;
    subtitle: string;
    steps: { num: string; title: string; description: string }[];
  };
  useCases: {
    eyebrow: string;
    title: string;
    subtitle: string;
    items: { icon: string; title: string; description: string }[];
  };
  hardware: {
    eyebrow: string;
    title: string;
    subtitle: string;
    primaryVendor: string;
    primaryVendorNote: string;
    roadmapNote: string;
  };
  pricing: {
    eyebrow: string;
    title: string;
    subtitle: string;
    plans: {
      name: string;
      price: string;
      period: string;
      tagline: string;
      cta: string;
      highlight: boolean;
      features: string[];
      badge?: string;
    }[];
    currencyNote: string;
  };
  testimonials: {
    eyebrow: string;
    title: string;
    subtitle: string;
    placeholder: string;
    valueProps: { icon: string; title: string; description: string }[];
  };
  faq: {
    eyebrow: string;
    title: string;
    subtitle: string;
    items: { q: string; a: string }[];
  };
  finalCta: {
    title: string;
    subtitle: string;
    primary: string;
    secondary: string;
  };
  footer: {
    tagline: string;
    columns: { title: string; links: { label: string; href: string }[] }[];
    copyright: string;
    contact: string;
    location: string;
  };
}

const fr: LandingCopy = {
  header: {
    brand: "MikCloud",
    nav: {
      features: "Fonctionnalités",
      benefits: "Bénéfices",
      pricing: "Tarifs",
      faq: "FAQ",
    },
    signIn: "Se connecter",
    signUp: "Créer mon compte",
    langLabel: "EN",
  },
  hero: {
    badge: "Prix Fondateur · 100 places seulement",
    title1: "Boostez vos revenus",
    titleAccent: "hotspot",
    title2: "depuis un seul cloud",
    subtitle:
      "MikCloud est la plateforme cloud de gestion de hotspot MikroTik, conçue en Afrique pour les opérateurs africains. Connectez votre premier routeur en 45 secondes.",
    ctaPrimary: "Se connecter à la console",
    ctaSecondary: "Voir la démo",
    ctaSignUp: "Créer mon compte",
    ctaDemoHint: "Compte démo : admin / admin123",
    trialHint: "Essai gratuit 90 jours · 1 routeur · sans carte bancaire",
    statBar: [
      { value: "45 s", label: "pour connecter un routeur" },
      { value: "0", label: "IP publique requise" },
      { value: "6+", label: "mobile money supportés" },
      { value: "14+", label: "pays africains couverts" },
    ],
  },
  trust: {
    items: [
      { icon: "Timer", label: "45 s pour connecter un routeur" },
      { icon: "ShieldOff", label: "Zéro IP publique, zéro port ouvert" },
      { icon: "Wallet", label: "Wave, Orange Money, MTN MoMo, Moov, MPesa, Airtel Money" },
      { icon: "Globe", label: "Conçu en Afrique, hébergé dans le cloud UE" },
    ],
  },
  benefits: {
    eyebrow: "Bénéfices",
    title: "Cloud-native, sécurisé et souverain",
    subtitle:
      "MikCloud déploie une architecture cloud-native Zero Trust à travers tous vos sites. Une seule console pour piloter tous vos routeurs, où qu'ils soient — avec conformité aux réglementations locales de données.",
    items: [
      {
        icon: "Cloud",
        title: "Cloud-native & sécurisé",
        description:
          "Architecture 100 % cloud, chiffrement TLS en transit et au repos. Authentification Zero Trust, mots de passe hashés (bcrypt), identifiants routeur chiffrés. Aucun agent à installer sur le routeur.",
      },
      {
        icon: "MonitorSmartphone",
        title: "Une console pour tous vos routeurs",
        description:
          "Pilotez 1, 10 ou 100 routeurs MikroTik depuis une seule console. Vue consolidée multi-sites : sessions, ventes, revenus par site. Plus besoin de jongler entre Winbox, NAT et DDNS.",
      },
      {
        icon: "Rocket",
        title: "Déploiement facile & scalable",
        description:
          "Infrastructure-agnostic : votre routeur appelle le cloud en sortant uniquement. Ajoutez un routeur en 45 s, scalez à 100 sans rien reconfigurer. Ça marche derrière n'importe quel opérateur, même en CGNAT.",
      },
      {
        icon: "ShieldCheck",
        title: "Conforme & souverain",
        description:
          "Données hébergées dans l'UE (Francfort) avec rétention conforme aux réglementations locales. Journaux d'activité en temps réel, export et suppression de vos données à tout moment. Souveraineté numérique respectée.",
      },
    ],
  },
  features: {
    eyebrow: "Fonctionnalités",
    title: "Tout votre métier hotspot, un seul écran",
    subtitle:
      "Du voucher à l'impression A4, de la session temps réel au portefeuille revendeur — MikCloud réunit ce que vous faisiez à la main dans Winbox, Excel et WhatsApp.",
    items: [
      {
        icon: "Ticket",
        title: "Vouchers A4 + QR",
        description:
          "Génération par lots (1 à 500), préfixe et longueur de code personnalisables, impression tickets prédécoupés avec QR code. Traçabilité complète par lot : site émetteur, canal, revendeur, statut voucher par voucher.",
        cta: "Générer un lot",
      },
      {
        icon: "Activity",
        title: "Sessions temps réel",
        description:
          "Table de sessions live rafraîchie toutes les 5 s, déconnexion (kick) instantanée, vue multi-routeurs consolidée. CPU, uptime, statut de chaque routeur MikroTik d'un coup d'œil.",
        cta: "Voir les sessions",
      },
      {
        icon: "Network",
        title: "Multi-sites & multi-routeurs",
        description:
          "Un compte, N hotspots, N routeurs. Vue d'ensemble multi-sites : sessions, ventes et revenus par site. Idéal pour les gérants multi-sites, ISPs et revendeurs qui jonglent entre Winbox, NAT et DDNS.",
        cta: "Gérer plusieurs sites",
      },
      {
        icon: "Wallet",
        title: "Revendeurs à portefeuille",
        description:
          "Créez des comptes revendeurs avec portefeuille de crédits, rechargements traçables et journal des transactions. Chaque revendeur suit ses ventes, vous gardez la vue d'ensemble.",
        cta: "Créer un revendeur",
      },
      {
        icon: "BarChart3",
        title: "Rapports & comptabilité",
        description:
          "Comptabilité multi-sites : ventes par jour, semaine, mois et par routeur (part de CA, panier moyen). Activité : revenus, ventes par profil, trafic consommé, statut des vouchers.",
        cta: "Consulter les rapports",
      },
      {
        icon: "Bell",
        title: "Alertes & notifications",
        description:
          "Surveillance automatique : auto-marquage hors ligne des routeurs (3 × 45 s sans check-in), alertes de stock de vouchers bas, rapports journaliers. Canaux Telegram, WhatsApp, email.",
        cta: "Configurer une alerte",
      },
    ],
  },
  how: {
    eyebrow: "Comment ça marche",
    title: "En ligne en 3 étapes, moins de 5 minutes",
    subtitle:
      "Aucune installation réseau. Aucune IP publique. Aucun port à ouvrir. Le routeur appelle le cloud dans le sens sortant uniquement — ça marche partout, même derrière CGNAT.",
    steps: [
      {
        num: "01",
        title: "Connectez votre routeur MikroTik",
        description:
          "Ajoutez votre routeur dans MikCloud : IP, port 8728, identifiants API. Ou collez le script .rsc auto-installable depuis Winbox — le routeur appelle le cloud toutes les 45 s.",
      },
      {
        num: "02",
        title: "Configurez vos forfaits & vouchers",
        description:
          "Débit (format RouterOS), durée de session, validité, quota Go, prix. Générez un lot de vouchers, débitez automatiquement le portefeuille du revendeur, imprimez en A4 prédécoupé.",
      },
      {
        num: "03",
        title: "Vendez & supervisez",
        description:
          "Acceptez Wave, Orange Money, MTN MoMo, Moov, MPesa ou Airtel Money. Suivez les sessions en temps réel, consultez les rapports par site, par profil et par période — depuis n'importe quel navigateur.",
      },
    ],
  },
  useCases: {
    eyebrow: "Cas d'usage",
    title: "Conçu pour tous les opérateurs WiFi africains",
    subtitle:
      "Du cybercafé de quartier au WISP multi-ville, en passant par l'hôtel, le campus et le marché de rue — MikCloud s'adapte à votre métier.",
    items: [
      {
        icon: "Monitor",
        title: "Cybercafés & boutiques internet",
        description:
          "Vendez l'accès à la minute, au quart d'heure ou à l'heure. Vouchers prépayés, impression à la volée, comptabilité journalière automatique.",
      },
      {
        icon: "Hotel",
        title: "Hôtels & résidences",
        description:
          "Vouchers invités remis à la réception, quotas par chambre, forfaits jour / séjour. Comptabilité par étage ou par bâtiment.",
      },
      {
        icon: "GraduationCap",
        title: "Campus & écoles",
        description:
          "Quotas étudiants par semestre, profils par classe, sessions limitées en temps ou en volume. Rapports d'usage par promotion.",
      },
      {
        icon: "Server",
        title: "ISPs & WISP locaux",
        description:
          "Multi-sites, revendeurs à portefeuille, alertes automatiques. Pilotez 1, 10 ou 100 routeurs depuis une seule console cloud.",
      },
      {
        icon: "Coffee",
        title: "Restaurants & cafés",
        description:
          "WiFi client gratuit avec capture d'email pour le marketing, ou WiFi premium payant au-delà d'un quota. Bascule en 1 clic.",
      },
      {
        icon: "Store",
        title: "Marchés & gares",
        description:
          "Vouchers en gros confiés à des revendeurs de rue. Chaque revendeur suit son stock et ses ventes, vous gardez la traçabilité complète.",
      },
    ],
  },
  hardware: {
    eyebrow: "Compatibilité",
    title: "Conçu pour MikroTik RouterOS",
    subtitle:
      "MikCloud parle le protocole binaire natif RouterOS (API port 8728) — le standard des hotspots professionnels en Afrique. Login v6.43+ et fallback challenge MD5 gérés automatiquement.",
    primaryVendor: "MikroTik",
    primaryVendorNote:
      "Tout routeur sous RouterOS 6.43+ est supporté : hAP, hEX, RB, CCR, etc. Mode Simulé intégré pour démontrer sans matériel.",
    roadmapNote:
      "Roadmap : support Ubiquiti UniFi, TP-Link et Cisco prévu — MikCloud évolue vers une plateforme multi-vendor.",
  },
  pricing: {
    eyebrow: "Tarifs",
    title: "Un prix clair, sans surprise",
    subtitle:
      "Tarif en FCFA (UEMOA et CEMAC). Pour NGN, GHS, KES, TZS, UGX, ZAR et autres devises africaines, le montant est converti automatiquement à la souscription.",
    plans: [
      {
        name: "Essentiel",
        price: "1 250 FCFA",
        period: "/ mois / routeur",
        tagline: "Pour démarrer, sans engagement.",
        cta: "Commencer",
        highlight: false,
        features: [
          "1 routeur (ajoutez-en à la demande)",
          "Vouchers, sessions, profils",
          "Rapports par routeur",
          "Support email",
          "Sans engagement, annulable à tout moment",
        ],
      },
      {
        name: "Illimité",
        price: "12 000 FCFA",
        period: "/ an · routeurs illimités",
        tagline: "Pour les multi-sites et ISPs.",
        cta: "Réserver ma place Fondateur",
        highlight: true,
        badge: "Prix Fondateur · bloqué à vie",
        features: [
          "Routeurs illimités",
          "Revendeurs illimités",
          "Multi-sites avec vue consolidée",
          "Alertes Telegram, WhatsApp, email",
          "Rapports comptables multi-sites",
          "Support prioritaire WhatsApp",
          "Tarif bloqué à vie (Prix Fondateur)",
        ],
      },
    ],
    currencyNote:
      "Devises supportées : FCFA (UEMOA : CI, SN, ML, BF, BJ, TG, NE + CEMAC : CM, GA, CG, TD, CF, GQ), NGN (Nigeria), GHS (Ghana), KES (Kenya), TZS (Tanzanie), UGX (Ouganda), ZAR (Afrique du Sud).",
  },
  testimonials: {
    eyebrow: "Témoignages",
    title: "Conçu avec les opérateurs, pour les opérateurs",
    subtitle:
      "MikCloud est en lancement — les premiers témoignages clients arriveront ici. En attendant, voici ce que la plateforme vous apporte dès le premier jour.",
    placeholder: "Premiers témoignages clients à venir",
    valueProps: [
      {
        icon: "Clock",
        title: "45 secondes",
        description: "C'est le temps moyen pour connecter un routeur MikroTik à MikCloud, script .rsc inclus.",
      },
      {
        icon: "TrendingDown",
        title: "−20 % à −92 %",
        description: "L'économie réalisée en passant au forfait Illimité : de 1 à 10 routeurs, le coût marginal tombe à presque zéro.",
      },
      {
        icon: "Lock",
        title: "Aucune IP publique",
        description: "Le routeur appelle le cloud en sortant uniquement. Aucun port à ouvrir, aucun VPN, aucune exposition internet.",
      },
      {
        icon: "Headphones",
        title: "Support local",
        description: "Assistance en français et en anglais, par WhatsApp et email — depuis Abidjan, pour toute l'Afrique.",
      },
    ],
  },
  faq: {
    eyebrow: "FAQ",
    title: "Questions fréquentes",
    subtitle:
      "Tout ce que les opérateurs africains nous demandent avant de se lancer.",
    items: [
      {
        q: "MikCloud marche-t-il derrière Orange, MTN, Moov, Safaricom ou Airtel (CGNAT) ?",
        a: "Oui — c'est précisément pour ça que MikCloud existe. Le routeur appelle le cloud toutes les 45 secondes dans le sens sortant uniquement. Aucune IP publique, aucun port à ouvrir, aucun VPN. Ça marche derrière n'importe quel opérateur africain en CGNAT : Orange CI/CM/SN/ML/BF/BJ/TG/NE, MTN (15 pays), Moov CI/BF/BJ, Safaricom KE/TZ, Airtel (14 pays), Vodafone GH, Surfline GH, 9Mobile et Glo NG, etc.",
      },
      {
        q: "Dois-je avoir une IP publique ou ouvrir un port sur mon routeur ?",
        a: "Non. MikCloud fonctionne entièrement en sortant : le routeur MikroTik établit la connexion vers le cloud. Vous n'avez besoin ni d'IP publique, ni de DDNS, ni d'ouverture de port. C'est ce qui rend la plateforme utilisable immédiatement, même sur une connexion Orange ou MTN grand public.",
      },
      {
        q: "Quels routeurs sont supportés ?",
        a: "MikCloud s'appuie sur le protocole binaire natif RouterOS (API port 8728). Tout routeur MikroTik sous RouterOS 6.43+ est supporté — hAP, hEX, RB, CCR, etc. Le login v6.43+ et le fallback challenge MD5 sont gérés automatiquement. Un mode Simulé intégré permet aussi de démontrer toute la plateforme sans matériel. Le support Ubiquiti, TP-Link et Cisco est en roadmap.",
      },
      {
        q: "Quels moyens de paiement puis-je accepter auprès de mes clients ?",
        a: "MikCloud génère des liens de paiement pré-composés au montant exact, compatibles avec Wave (SN/CI), Orange Money (UEMOA + CEEAC), MTN MoMo (15 pays), Moov Money (UEMOA), MPesa (KE/TZ/CD/GH) et Airtel Money (14 pays). Vous pouvez aussi encaisser en espèces et valider manuellement. Côté abonnement MikCloud, le paiement se fait par mobile money ou virement.",
      },
      {
        q: "Puis-je essayer avant de payer ?",
        a: "Oui. Le compte démo (admin / admin123) vous donne accès à toute la console avec un routeur simulé. Pour connecter un vrai routeur MikroTik et juger par vous-même, souscrivez au forfait Essentiel (1 250 F/mois, sans engagement) — annulable à tout moment. Le forfait Illimité Prix Fondateur est réservé aux 100 premiers.",
      },
      {
        q: "Mes données sont-elles en sécurité ? Où sont-elles stockées ?",
        a: "Vos données vivent dans une base PostgreSQL managée (Neon), chiffrée en transit (TLS) et au repos. Le backend Go est hébergé sur Render dans l'UE (Francfort). Vos identifiants de routeur sont stockés chiffrés, vos mots de passe utilisateur sont hashés (bcrypt). Vous pouvez exporter ou supprimer vos données à tout moment. Conformité aux réglementations locales de données respectée.",
      },
    ],
  },
  finalCta: {
    title: "Prêt à connecter votre premier routeur ?",
    subtitle:
      "Votre routeur MikroTik en ligne en 45 secondes. Sans IP publique. Sans engagement. Sans installer quoi que ce soit sur votre réseau.",
    primary: "Se connecter à la console",
    secondary: "Réserver ma place Fondateur",
  },
  footer: {
    tagline:
      "La plateforme cloud de gestion de hotspot MikroTik, conçue en Afrique pour les opérateurs africains.",
    columns: [
      {
        title: "Produit",
        links: [
          { label: "Bénéfices", href: "#benefits" },
          { label: "Fonctionnalités", href: "#features" },
          { label: "Tarifs", href: "#pricing" },
          { label: "Cas d'usage", href: "#use-cases" },
          { label: "FAQ", href: "#faq" },
        ],
      },
      {
        title: "Société",
        links: [
          { label: "À propos", href: "#" },
          { label: "Contact", href: "mailto:freelancetechnologies.ci@gmail.com" },
          { label: "Blog", href: "#" },
          { label: "Partenaires", href: "#" },
        ],
      },
      {
        title: "Ressources",
        links: [
          { label: "Documentation", href: "#" },
          { label: "Statut", href: "#" },
          { label: "Changelog", href: "#" },
          { label: "API", href: "#" },
        ],
      },
    ],
    copyright: "© 2025 MikCloud — ftechnologies18. Tous droits réservés.",
    contact: "freelancetechnologies.ci@gmail.com",
    location: "Abidjan, Côte d'Ivoire · Afrique de l'Ouest, Centrale et de l'Est",
  },
};

const en: LandingCopy = {
  header: {
    brand: "MikCloud",
    nav: {
      features: "Features",
      benefits: "Benefits",
      pricing: "Pricing",
      faq: "FAQ",
    },
    signIn: "Sign in",
    signUp: "Sign up",
    langLabel: "FR",
  },
  hero: {
    badge: "Founder Price · 100 spots only",
    title1: "Boost your",
    titleAccent: "hotspot",
    title2: "revenue from one cloud",
    subtitle:
      "MikCloud is the cloud-managed MikroTik hotspot platform built in Africa for African operators. Connect your first router in 45 seconds.",
    ctaPrimary: "Sign in to console",
    ctaSecondary: "Try the demo",
    ctaSignUp: "Create account",
    ctaDemoHint: "Demo account: admin / admin123",
    trialHint: "90-day free trial · 1 router · no credit card",
    statBar: [
      { value: "45s", label: "to connect a router" },
      { value: "0", label: "public IP required" },
      { value: "6+", label: "mobile money supported" },
      { value: "14+", label: "African countries covered" },
    ],
  },
  trust: {
    items: [
      { icon: "Timer", label: "45s to connect a router" },
      { icon: "ShieldOff", label: "Zero public IP, zero open port" },
      { icon: "Wallet", label: "Wave, Orange Money, MTN MoMo, Moov, MPesa, Airtel Money" },
      { icon: "Globe", label: "Built in Africa, cloud-hosted in the EU" },
    ],
  },
  benefits: {
    eyebrow: "Benefits",
    title: "Cloud-native, secure and sovereign",
    subtitle:
      "MikCloud deploys a cloud-native Zero Trust architecture across all your sites. One console to pilot all your routers, wherever they are — with compliance to local data regulations.",
    items: [
      {
        icon: "Cloud",
        title: "Cloud-native & secure",
        description:
          "100% cloud architecture, TLS encryption in transit and at rest. Zero Trust authentication, hashed passwords (bcrypt), encrypted router credentials. No agent to install on the router.",
      },
      {
        icon: "MonitorSmartphone",
        title: "One console for all routers",
        description:
          "Pilot 1, 10 or 100 MikroTik routers from a single console. Consolidated multi-site view: sessions, sales, revenue per site. No more juggling Winbox, NAT and DDNS.",
      },
      {
        icon: "Rocket",
        title: "Easy to deploy & scalable",
        description:
          "Infrastructure-agnostic: your router calls the cloud outbound only. Add a router in 45s, scale to 100 without reconfiguring anything. Works behind any carrier, even in CGNAT.",
      },
      {
        icon: "ShieldCheck",
        title: "Compliant & sovereign",
        description:
          "Data hosted in the EU (Frankfurt) with retention compliant to local regulations. Real-time activity logs, export and delete your data anytime. Digital sovereignty respected.",
      },
    ],
  },
  features: {
    eyebrow: "Features",
    title: "Your whole hotspot business, one screen",
    subtitle:
      "From vouchers to A4 printing, from real-time sessions to reseller wallets — MikCloud unifies what you used to juggle across Winbox, Excel and WhatsApp.",
    items: [
      {
        icon: "Ticket",
        title: "A4 vouchers + QR",
        description:
          "Batch generation (1 to 500), custom code prefix and length, pre-cut A4 ticket printing with QR codes. Full batch traceability: issuing site, channel, reseller, per-voucher status.",
        cta: "Generate a batch",
      },
      {
        icon: "Activity",
        title: "Real-time sessions",
        description:
          "Live session table refreshed every 5s, instant kick, consolidated multi-router view. CPU, uptime and status of every MikroTik router at a glance.",
        cta: "View sessions",
      },
      {
        icon: "Network",
        title: "Multi-site & multi-router",
        description:
          "One account, N hotspots, N routers. Multi-site overview: sessions, sales and revenue per site. Built for multi-site operators, ISPs and resellers juggling Winbox, NAT and DDNS.",
        cta: "Manage multiple sites",
      },
      {
        icon: "Wallet",
        title: "Resellers with wallets",
        description:
          "Create reseller accounts with credit wallets, traceable top-ups and a transaction journal. Each reseller tracks their own sales — you keep the bird's-eye view.",
        cta: "Create a reseller",
      },
      {
        icon: "BarChart3",
        title: "Reports & accounting",
        description:
          "Multi-site accounting: sales by day, week, month and per router (revenue share, average basket). Activity: revenue, sales by profile, traffic consumed, voucher status.",
        cta: "Open reports",
      },
      {
        icon: "Bell",
        title: "Alerts & notifications",
        description:
          "Automatic monitoring: offline router auto-marking (3 × 45s without check-in), low voucher stock alerts, daily reports. Channels: Telegram, WhatsApp, email.",
        cta: "Set up an alert",
      },
    ],
  },
  how: {
    eyebrow: "How it works",
    title: "Online in 3 steps, under 5 minutes",
    subtitle:
      "No network setup. No public IP. No port to open. The router calls the cloud outbound only — it works everywhere, even behind CGNAT.",
    steps: [
      {
        num: "01",
        title: "Connect your MikroTik router",
        description:
          "Add your router to MikCloud: IP, port 8728, API credentials. Or paste the auto-installable .rsc script from Winbox — the router calls the cloud every 45s.",
      },
      {
        num: "02",
        title: "Set up plans & vouchers",
        description:
          "Bandwidth (RouterOS format), session duration, validity, data quota, price. Generate a voucher batch, automatically debit the reseller wallet, print pre-cut A4.",
      },
      {
        num: "03",
        title: "Sell & monitor",
        description:
          "Accept Wave, Orange Money, MTN MoMo, Moov, MPesa or Airtel Money. Track sessions in real time, read reports by site, profile and period — from any browser.",
      },
    ],
  },
  useCases: {
    eyebrow: "Use cases",
    title: "Built for every African WiFi operator",
    subtitle:
      "From the corner cybercafé to the multi-city WISP, through hotels, campuses and street markets — MikCloud adapts to your business.",
    items: [
      {
        icon: "Monitor",
        title: "Cybercafés & internet shops",
        description:
          "Sell access by the minute, quarter-hour or hour. Prepaid vouchers, on-the-fly printing, automatic daily accounting.",
      },
      {
        icon: "Hotel",
        title: "Hotels & residences",
        description:
          "Guest vouchers handed out at reception, per-room quotas, day / stay packages. Accounting by floor or building.",
      },
      {
        icon: "GraduationCap",
        title: "Campuses & schools",
        description:
          "Per-semester student quotas, per-class profiles, time- or volume-limited sessions. Usage reports by promotion.",
      },
      {
        icon: "Server",
        title: "Local ISPs & WISPs",
        description:
          "Multi-site, resellers with wallets, automatic alerts. Pilot 1, 10 or 100 routers from a single cloud console.",
      },
      {
        icon: "Coffee",
        title: "Restaurants & cafés",
        description:
          "Free guest WiFi with email capture for marketing, or premium paid WiFi above a quota. One-click toggle.",
      },
      {
        icon: "Store",
        title: "Markets & stations",
        description:
          "Bulk vouchers handed to street resellers. Each reseller tracks their stock and sales, you keep full traceability.",
      },
    ],
  },
  hardware: {
    eyebrow: "Compatibility",
    title: "Built for MikroTik RouterOS",
    subtitle:
      "MikCloud speaks the native RouterOS binary protocol (API port 8728) — the standard for professional African hotspots. Login v6.43+ and MD5 challenge fallback handled automatically.",
    primaryVendor: "MikroTik",
    primaryVendorNote:
      "Any router running RouterOS 6.43+ is supported: hAP, hEX, RB, CCR, etc. Built-in Simulated mode to demo without hardware.",
    roadmapNote:
      "Roadmap: Ubiquiti UniFi, TP-Link and Cisco support planned — MikCloud evolves toward a multi-vendor platform.",
  },
  pricing: {
    eyebrow: "Pricing",
    title: "Clear pricing, no surprise",
    subtitle:
      "Prices in FCFA (UEMOA and CEMAC). For NGN, GHS, KES, TZS, UGX, ZAR and other African currencies, the amount is auto-converted at checkout.",
    plans: [
      {
        name: "Essential",
        price: "1,250 FCFA",
        period: "/ month / router",
        tagline: "To get started, no commitment.",
        cta: "Get started",
        highlight: false,
        features: [
          "1 router (add more on demand)",
          "Vouchers, sessions, profiles",
          "Per-router reports",
          "Email support",
          "No commitment, cancel anytime",
        ],
      },
      {
        name: "Unlimited",
        price: "12,000 FCFA",
        period: "/ year · unlimited routers",
        tagline: "For multi-site and ISPs.",
        cta: "Claim my Founder spot",
        highlight: true,
        badge: "Founder Price · locked for life",
        features: [
          "Unlimited routers",
          "Unlimited resellers",
          "Multi-site with consolidated view",
          "Telegram, WhatsApp, email alerts",
          "Multi-site accounting reports",
          "Priority WhatsApp support",
          "Price locked for life (Founder)",
        ],
      },
    ],
    currencyNote:
      "Supported currencies: FCFA (UEMOA: CI, SN, ML, BF, BJ, TG, NE + CEMAC: CM, GA, CG, TD, CF, GQ), NGN (Nigeria), GHS (Ghana), KES (Kenya), TZS (Tanzania), UGX (Uganda), ZAR (South Africa).",
  },
  testimonials: {
    eyebrow: "Testimonials",
    title: "Built with operators, for operators",
    subtitle:
      "MikCloud is launching — first customer testimonials will appear here. Meanwhile, here's what the platform brings you from day one.",
    placeholder: "First customer testimonials coming soon",
    valueProps: [
      {
        icon: "Clock",
        title: "45 seconds",
        description: "The average time to connect a MikroTik router to MikCloud, .rsc script included.",
      },
      {
        icon: "TrendingDown",
        title: "−20% to −92%",
        description: "The savings when moving to the Unlimited plan: from 1 to 10 routers, marginal cost drops near zero.",
      },
      {
        icon: "Lock",
        title: "No public IP",
        description: "The router calls the cloud outbound only. No port to open, no VPN, no internet exposure.",
      },
      {
        icon: "Headphones",
        title: "Local support",
        description: "Support in French and English, via WhatsApp and email — from Abidjan, for all of Africa.",
      },
    ],
  },
  faq: {
    eyebrow: "FAQ",
    title: "Frequently asked questions",
    subtitle: "Everything African operators ask us before getting started.",
    items: [
      {
        q: "Does MikCloud work behind Orange, MTN, Moov, Safaricom or Airtel (CGNAT)?",
        a: "Yes — that's exactly why MikCloud exists. The router calls the cloud every 45 seconds, outbound only. No public IP, no port to open, no VPN. It works behind any African carrier in CGNAT: Orange CI/CM/SN/ML/BF/BJ/TG/NE, MTN (15 countries), Moov CI/BF/BJ, Safaricom KE/TZ, Airtel (14 countries), Vodafone GH, Surfline GH, 9Mobile and Glo NG, etc.",
      },
      {
        q: "Do I need a public IP or to open a port on my router?",
        a: "No. MikCloud works entirely outbound: the MikroTik router initiates the connection to the cloud. You don't need a public IP, DDNS or port forwarding. That's what makes the platform usable immediately, even on a consumer Orange or MTN line.",
      },
      {
        q: "Which routers are supported?",
        a: "MikCloud uses the native RouterOS binary protocol (API port 8728). Any MikroTik router running RouterOS 6.43+ is supported — hAP, hEX, RB, CCR, etc. Login v6.43+ and MD5 challenge fallback are handled automatically. A built-in Simulated mode also lets you demo the whole platform without hardware. Ubiquiti, TP-Link and Cisco support is on the roadmap.",
      },
      {
        q: "Which payment methods can I accept from my customers?",
        a: "MikCloud generates pre-composed payment links at the exact amount, compatible with Wave (SN/CI), Orange Money (UEMOA + CEEAC), MTN MoMo (15 countries), Moov Money (UEMOA), MPesa (KE/TZ/CD/GH) and Airtel Money (14 countries). You can also collect cash and validate manually. For the MikCloud subscription itself, payment is via mobile money or bank transfer.",
      },
      {
        q: "Can I try before I pay?",
        a: "Yes. The demo account (admin / admin123) gives you full console access with a simulated router. To connect a real MikroTik router and judge for yourself, subscribe to the Essential plan (1,250 F/month, no commitment) — cancel anytime. The Founder Unlimited plan is reserved for the first 100.",
      },
      {
        q: "Are my data safe? Where are they stored?",
        a: "Your data lives in a managed PostgreSQL database (Neon), encrypted in transit (TLS) and at rest. The Go backend is hosted on Render in the EU (Frankfurt). Your router credentials are stored encrypted, your user passwords are hashed (bcrypt). You can export or delete your data at any time. Compliance with local data regulations is respected.",
      },
    ],
  },
  finalCta: {
    title: "Ready to connect your first router?",
    subtitle:
      "Your MikroTik router online in 45 seconds. No public IP. No commitment. Nothing to install on your network.",
    primary: "Sign in to console",
    secondary: "Claim my Founder spot",
  },
  footer: {
    tagline:
      "The cloud-managed MikroTik hotspot platform, built in Africa for African operators.",
    columns: [
      {
        title: "Product",
        links: [
          { label: "Benefits", href: "#benefits" },
          { label: "Features", href: "#features" },
          { label: "Pricing", href: "#pricing" },
          { label: "Use cases", href: "#use-cases" },
          { label: "FAQ", href: "#faq" },
        ],
      },
      {
        title: "Company",
        links: [
          { label: "About", href: "#" },
          { label: "Contact", href: "mailto:freelancetechnologies.ci@gmail.com" },
          { label: "Blog", href: "#" },
          { label: "Partners", href: "#" },
        ],
      },
      {
        title: "Resources",
        links: [
          { label: "Documentation", href: "#" },
          { label: "Status", href: "#" },
          { label: "Changelog", href: "#" },
          { label: "API", href: "#" },
        ],
      },
    ],
    copyright: "© 2025 MikCloud — ftechnologies18. All rights reserved.",
    contact: "freelancetechnologies.ci@gmail.com",
    location: "Abidjan, Côte d'Ivoire · West, Central and East Africa",
  },
};

export const landingCopy: Record<Lang, LandingCopy> = { fr, en };
