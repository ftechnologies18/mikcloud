// Landing page MikCloud « Clay » — copie marketing bilingue FR/EN.
//
// Auto-contenue (à part du dictionnaire applicatif i18n) : la vitrine a un
// volume de copie marketing important qui n'a pas vocation à vivre dans le
// dictionnaire applicatif. La langue courante est lue via le store zustand
// (useHotspotStore.lang) — pas de rechargement au switch.
//
// N°120 — refonte complète de la vitrine : MikCloud n'est plus seulement un
// gestionnaire Hotspot, c'est aussi un pare-feu cloud (4 protections posées
// sur le routeur : SafeWiFi, Shield, FamilyGuard, AntiVPN) et un outil de
// pilotage de parc (télémétrie, mise à jour RouterOS unitaire + flotte).
// La copie ne mentionne QUE des fonctionnalités réelles du produit — les
// chiffres mis en avant (500 vouchers/lot, 4 boucliers, 54 pays, 60 jours
// d'essai Hotspot) sont des constantes produit, pas des métriques d'usage inventées.
//
// N°124 — le mode résidentiel est renommé HomeNet (sécurité internet
// résidentiel) partout où le mode est nommé ; la clientèle cible du mode
// Hotspot (hôtels, cybercafés, maquis…) est exposée via le champ
// optionnel audience (rendu en chips clay), revendeurs mis en avant.
//
// Positionnement : marché africain pan-continental (UEMOA + CEMAC + Afrique
// de l'Est + Nigeria + Ghana). Multi mobile-money (Wave, Orange Money, MTN
// MoMo, Moov, MPesa, Airtel Money), multi-devises (FCFA, NGN, GHS, KES...).

export type Lang = "fr" | "en";

export interface LandingCopy {
  rail: {
    home: string;
    powers: string;
    protection: string;
    hotspot: string;
    fleet: string;
    pricing: string;
  };
  header: {
    brand: string;
    signIn: string;
    signUp: string;
    langLabel: string;
    homeLink: string;
  };
  hero: {
    badge: string;
    title1: string;
    titleAccent: string;
    title2: string;
    subtitle: string;
    ctaPrimary: string;
    ctaSecondary: string;
    trialHint: string;
    chips: string[];
  };
  marquee: string[];
  powers: {
    eyebrow: string;
    title: string;
    subtitle: string;
    cards: {
      tag: string;
      title: string;
      desc: string;
      items: string[];
    }[];
  };
  protection: {
    kicker: string;
    title: string;
    body: string;
    feats: { title: string; desc: string }[];
    panel: {
      title: string;
      scoreLabel: string;
      scoreVerdict: string;
      stats: { value: string; label: string }[];
      lines: { name: string; state: string }[];
      repairLine: { name: string; state: string };
    };
  };
  hotspot: {
    kicker: string;
    title: string;
    body: string;
    /** N°124 — clientèle cible du mode Hotspot + libellé du bandeau. */
    audience: string[];
    audienceLabel: string;
    feats: { title: string; desc: string }[];
    panel: {
      title: string;
      online: string;
      peak: string;
    };
  };
  fleet: {
    kicker: string;
    title: string;
    body: string;
    feats: { title: string; desc: string }[];
    panel: {
      title: string;
      rows: { name: string; version: string; state: string; upToDate: boolean }[];
      checkAll: string;
      updateAll: string;
    };
  };
  stats: { value: number; label: string }[];
  pricing: {
    eyebrow: string;
    title: string;
    subtitle: string;
    /** N°122 — aria-label du sélecteur de mode (Hotspot / HomeNet). */
    modesLabel: string;
    /** N°122 — tarifs segmentés par mode : chaque mode a ses 3 formules. */
    segments: {
      id: "hotspot" | "homenet";
      label: string;
      hint: string;
      /** N°124 — clientèle cible (chips clay) — uniquement le mode Hotspot. */
      audience?: string[];
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
    }[];
    currencyNote: string;
  };
  chat: {
    fabLabel: string;
    openLabel: string;
    closeLabel: string;
    /** N°131 — bulle d'invitation flottant à côté du bouton chat
        (animation « wahou » de découverte). */
    tease: string;
    title: string;
    statusBot: string;
    statusHuman: string;
    statusClosed: string;
    placeholder: string;
    send: string;
    humanBtn: string;
    newConv: string;
    handoffNote: string;
    closedNote: string;
    suggestions: string[];
    agentBadge: string;
    inputLabel: string;
    sendError: string;
  };
  finalCta: {
    kicker: string;
    title: string;
    subtitle: string;
    primary: string;
    secondary: string;
  };
  footer: {
    tagline: string;
    columns: { title: string; links: { label: string; href: string }[] }[];
    /** N°70 — libellé du lien vers /legal/confidentialite. */
    legal: string;
  };
}

const fr: LandingCopy = {
  rail: {
    home: "Accueil",
    powers: "Super-pouvoirs",
    protection: "Protection",
    hotspot: "Hotspot",
    fleet: "Parc routeurs",
    pricing: "Tarifs",
  },
  header: {
    brand: "MikCloud",
    signIn: "Se connecter",
    signUp: "Essai gratuit",
    langLabel: "EN",
    homeLink: "Retour à l'accueil",
  },
  hero: {
    badge: "Gestion Hotspot · Pare-feu HomeNet · Cloud MikroTik",
    title1: "Votre WiFi,",
    titleAccent: "blindé",
    title2: "par le cloud.",
    subtitle:
      "Que vous exploitiez un hotspot public ou protégiez votre domicile avec HomeNet, MikCloud réunit vouchers, portails captifs à votre marque, quatre boucliers pare-feu et pilotage complet de votre parc MikroTik — une seule console, réparée automatiquement depuis le cloud.",
    ctaPrimary: "Protéger mon réseau",
    ctaSecondary: "Découvrir la plateforme",
    trialHint: "Essai gratuit · sans carte bancaire — 60 jours Hotspot · 30 jours HomeNet",
    chips: ["4/4 protections actives", "500 vouchers par lot", "Agent · check-in 45 s"],
  },
  marquee: [
    "Vouchers & codes d'accès",
    "Filtrage DNS Quad9",
    "Anti-piratage WiFi",
    "Couvre-feu internet",
    "HomeNet · Sécurité internet résidentiel",
    "Bloque-VPN",
    "QoS & forfait FAI",
    "Mode Vente revendeurs",
    "Mise à jour RouterOS",
    "Portail captif à votre marque",
    "WiFi jetable par QR",
    "Notifications Telegram",
    "Paiement Wave",
  ],
  powers: {
    eyebrow: "Une console, trois super-pouvoirs",
    title: "Tout ce qu'il faut pour régner sur votre réseau.",
    subtitle:
      "Gestion hotspot, protection cloud et pilotage du parc : MikCloud réunit en une seule interface tout ce que votre infrastructure réclame — sans Winbox, sans serveur à maintenir.",
    cards: [
      {
        tag: "Fondation",
        title: "Gestion Hotspot",
        desc: "Le cœur historique de MikCloud, devenu adulte : portails captifs à votre marque, vouchers par lots, quotas et bridage au forfait, sessions en direct.",
        items: [
          "Portail captif 100 % à votre marque",
          "Vouchers par lots jusqu'à 500",
          "Quota, débit et durée par forfait",
          "Mode Vente & revendeurs avec PIN",
        ],
      },
      {
        tag: "4 boucliers",
        title: "Protection Cloud",
        desc: "Quatre protections pare-feu posées directement sur vos routeurs, pilotées et auto-réparées depuis le cloud toutes les six heures.",
        items: [
          "SafeWiFi — filtrage DNS Quad9 / AdGuard",
          "Shield — ports d'administration blindés",
          "FamilyGuard — couvre-feu horaire",
          "AntiVPN — tunnels coupés, WhatsApp intact",
        ],
      },
      {
        tag: "Parc",
        title: "Pilotage du parc",
        desc: "Télémétrie live, mise à jour RouterOS en un clic — routeur par routeur ou tout le parc d'un geste — et qualité de ligne mesurée en continu.",
        items: [
          "Mise à jour RouterOS sans Winbox",
          "Télémétrie CPU, mémoire, uptime",
          "QoS agrégée & forfait FAI",
          "Docteur Pool IP & auto-réparation",
        ],
      },
    ],
  },
  protection: {
    kicker: "Protection cloud",
    title: "Le bouclier qui veille pendant que vous dormez.",
    body: "Chaque protection est vérifiée par signatures et réparée automatiquement toutes les six heures. Sans rien installer, sans ouvrir Winbox : vos règles existantes sont préservées, et le WiFi reste opérationnel quoi qu'il arrive.",
    feats: [
      {
        title: "SafeWiFi — sites dangereux bloqués",
        desc: "Virus, arnaques et publicités stoppés au niveau DNS (Quad9 ou AdGuard Famille) — les échappatoires DoH et IPv6 sont fermées.",
      },
      {
        title: "Shield — anti-piratage du WiFi",
        desc: "Les ports d'administration de vos routeurs et le partage Windows deviennent inaccessibles depuis le WiFi public.",
      },
      {
        title: "FamilyGuard — couvre-feu internet",
        desc: "Tout l'internet des clients est coupé pendant la fenêtre programmée, par exemple de 22 h à 6 h.",
      },
      {
        title: "AntiVPN — tunnels coupés",
        desc: "WireGuard, OpenVPN, IPsec et Tor bloqués — les appels WhatsApp restent intacts.",
      },
    ],
    panel: {
      title: "Centre de protection",
      scoreLabel: "Score de protection",
      scoreVerdict: "Bien protégé",
      stats: [
        { value: "4", label: "protections actives" },
        { value: "6 h", label: "auto-réparation" },
        { value: "45 s", label: "check-in agent" },
      ],
      lines: [
        { name: "SafeWiFi · Filtrage DNS", state: "Actif" },
        { name: "Shield · Anti-piratage", state: "Actif" },
        { name: "FamilyGuard · Couvre-feu", state: "22:00 → 06:00" },
        { name: "AntiVPN · Bloque-VPN", state: "Actif" },
      ],
      repairLine: { name: "Auto-réparation cloud", state: "Vérifié il y a 2 min" },
    },
  },
  hotspot: {
    kicker: "Gestion hotspot",
    title: "Un portail captif dont vous serez fier.",
    body: "Offrez à vos clients une connexion fluide et soignée — à votre marque, sans aucune mention MikCloud. Vouchers à durée, quota ou multi-appareils : vous gardez le contrôle total. Vos revendeurs ? Ils vendent depuis leur téléphone : PWA protégée par PIN, stock transféré, ventes hors-ligne, rapport de journée.",
    audience: ["Hôtels", "Cybercafés", "Maquis", "Boutiques", "Campus", "Restaurants"],
    audienceLabel: "Pour qui ?",
    feats: [
      {
        title: "Marque 100 % personnalisable",
        desc: "Mode commercial avec grille tarifaire et paiement Wave, ou mode hospitalité avec promos et réseaux sociaux.",
      },
      {
        title: "Vouchers & forfaits",
        desc: "Durée, quota de données, débit, 1 à 10 appareils — et à l'épuisement : couper ou brider en douceur.",
      },
      {
        title: "Statistiques en direct",
        desc: "Affluence horaire, revenus, marge par profil, canaux directs et revendeurs.",
      },
    ],
    panel: {
      title: "Affluence horaire",
      online: "Heure de pointe · 19 h",
      peak: "24 h",
    },
  },
  fleet: {
    kicker: "Parc & flotte",
    title: "Votre parc MikroTik, sans quitter la console.",
    body: "L'agent MikCloud sort du routeur vers le cloud : il fonctionne derrière CGNAT, Orange ou Starlink, sans IP publique ni port ouvert. Puis chaque routeur se pilote à distance — jusqu'à la mise à jour RouterOS de toute la flotte.",
    feats: [
      {
        title: "Mise à jour RouterOS en 1 clic",
        desc: "Vérifiez la dernière version et installez-la routeur par routeur — ou tout le parc détecté en retard, d'un seul geste.",
      },
      {
        title: "Télémétrie en continu",
        desc: "CPU, mémoire, uptime, version RouterOS et qualité de ligne de chaque routeur, rafraîchis à chaque check-in.",
      },
      {
        title: "Alertes qui ne dorment pas",
        desc: "Routeur hors ligne, stock de vouchers bas, rapport quotidien : Telegram, WhatsApp ou e-mail.",
      },
    ],
    panel: {
      title: "Parc routeurs",
      rows: [
        { name: "Café du Plateau", version: "7.24.3", state: "À jour", upToDate: true },
        { name: "Hôtel Ébène", version: "7.16.2 → 7.24.3", state: "Mise à jour prête", upToDate: false },
        { name: "Cyber Marché", version: "7.24.3", state: "À jour", upToDate: true },
      ],
      checkAll: "Vérifier tout le parc",
      updateAll: "Mettre à jour le parc",
    },
  },
  stats: [
    { value: 500, label: "vouchers par lot" },
    { value: 4, label: "boucliers pare-feu" },
    { value: 54, label: "pays africains visés" },
    { value: 2, label: "modes — Hotspot & HomeNet" },
  ],
  pricing: {
    eyebrow: "Tarifs",
    title: "Deux modes, un nuage.",
    subtitle:
      "Exploitez un réseau public ou protégez votre foyer : MikCloud s'adapte. L'essai est offert dans les deux cas, sans engagement.",
    modesLabel: "Choisir votre mode",
    segments: [
      {
        id: "hotspot",
        label: "Hotspot",
        hint: "Hôtels, cybercafés, maquis, boutiques, campus… : vouchers, portail captif et vente d'accès avec vos revendeurs.",
        audience: ["Hôtels", "Cybercafés", "Maquis", "Boutiques", "Campus", "Restaurants"],
        plans: [
      {
        name: "Découverte",
        price: "0",
        period: "FCFA · 60 jours",
        tagline: "Pour découvrir MikCloud sans risque",
        cta: "Commencer gratuitement",
        highlight: false,
        features: [
          "1 routeur · toutes les fonctions",
          "4 protections incluses",
          "Mode Vente & revendeurs",
          "Sans carte bancaire",
        ],
      },
      {
        name: "Hotspot Annuel",
        price: "25 000",
        period: "FCFA / an",
        tagline: "Tous vos routeurs, un seul prix",
        cta: "Passer à l'annuel",
        highlight: true,
        badge: "Le plus choisi",
        features: [
          "Routeurs illimités",
          "4 protections sur tout le parc",
          "Mises à jour RouterOS de flotte",
          "Notifications Telegram & WhatsApp",
          "Support prioritaire",
        ],
      },
      {
        name: "Hotspot Mensuel",
        price: "2 500",
        period: "FCFA / mois / routeur",
        tagline: "Payez au fil de votre croissance",
        cta: "Choisir le mensuel",
        highlight: false,
        features: [
          "Par routeur actif",
          "Sans engagement",
          "Résiliable à tout moment",
          "Toutes les fonctions incluses",
        ],
      },
        ],
      },
      {
        id: "homenet",
        label: "HomeNet",
        hint: "Sécurité internet résidentiel : pare-feu cloud, filtrage DNS, couvre-feu familial et contrôle des appareils de votre foyer.",
        plans: [
          {
            name: "Essai HomeNet",
            price: "0",
            period: "FCFA · 30 jours",
            tagline: "Pour protéger votre famille sans risque",
            cta: "Commencer gratuitement",
            highlight: false,
            features: [
              "1 routeur · toutes les fonctions",
              "4 protections incluses",
              "Contrôle des appareils & pause dîner",
              "Sans carte bancaire",
            ],
          },
          {
            name: "HomeNet Annuel",
            price: "12 000",
            period: "FCFA / an",
            tagline: "Votre foyer protégé toute l'année",
            cta: "Passer à l'annuel",
            highlight: true,
            badge: "Le plus choisi",
            features: [
              "Routeurs illimités — toute la famille",
              "4 protections sur tout le parc",
              "Mises à jour RouterOS automatiques",
              "Notifications Telegram & WhatsApp",
              "Support prioritaire",
            ],
          },
          {
            name: "HomeNet Mensuel",
            price: "1 250",
            period: "FCFA / mois / routeur",
            tagline: "Protégez votre foyer sans engagement",
            cta: "Choisir le mensuel",
            highlight: false,
            features: [
              "Par routeur actif",
              "Sans engagement",
              "Résiliable à tout moment",
              "Toutes les fonctions incluses",
            ],
          },
        ],
      },
    ],
    currencyNote:
      "Frais de paiement répercutés sur le prix de liste : carte +6 %, Wave −3 % (remise mobile money). Essai offert : 60 jours en mode Hotspot, 30 jours en mode HomeNet.",
  },
  chat: {
    fabLabel: "Discuter avec l'assistant",
    openLabel: "Ouvrir la discussion",
    closeLabel: "Fermer la discussion",
    tease: "Une question ? Écrivez-la ici !",
    title: "Assistant MikCloud",
    statusBot: "Assistant · réponses instantanées",
    statusHuman: "Un conseiller vous répond",
    statusClosed: "Conversation clôturée",
    placeholder: "Écrivez votre message…",
    send: "Envoyer",
    humanBtn: "Parler à un humain",
    newConv: "Nouvelle conversation",
    handoffNote: "Transmis à l'équipe MikCloud — vos prochains messages partent directement au conseiller.",
    closedNote: "Cette conversation est terminée. À bientôt sur MikCloud !",
    suggestions: [
      "Hotspot ou HomeNet ?",
      "Quels sont les tarifs ?",
      "Quel routeur faut-il ?",
      "Essai gratuit ?",
    ],
    agentBadge: "Support",
    inputLabel: "Votre message",
    sendError: "Échec d'envoi — réessayez",
  },
  finalCta: {
    kicker: "Prêt·e à passer au niveau supérieur ?",
    title: "Blindez votre WiFi en moins de 5 minutes.",
    subtitle:
      "Créez votre compte, collez le script agent sur votre routeur, activez vos protections. Sans carte bancaire, sans serveur à maintenir.",
    primary: "Créer mon compte gratuit",
    secondary: "Se connecter",
  },
  footer: {
    tagline:
      "Le cloud qui protège : hotspot, pare-feu et pilotage MikroTik réunis dans un seul outil.",
    columns: [
      {
        title: "Produit",
        links: [
          { label: "Super-pouvoirs", href: "#pouvoirs" },
          { label: "Protection cloud", href: "#protection" },
          { label: "Hotspot", href: "#hotspot" },
          { label: "Parc routeurs", href: "#parc" },
          { label: "Tarifs", href: "#tarifs" },
        ],
      },
      {
        title: "Console",
        links: [
          { label: "Se connecter", href: "/login" },
          { label: "Mode Vente", href: "/sell" },
          { label: "Connexion WiFi jetable", href: "/wifi" },
        ],
      },
      {
        title: "Légal",
        links: [{ label: "Politique de confidentialité", href: "/legal/confidentialite" }],
      },
    ],
    legal: "Politique de confidentialité",
  },
};

const en: LandingCopy = {
  rail: {
    home: "Home",
    powers: "Superpowers",
    protection: "Protection",
    hotspot: "Hotspot",
    fleet: "Router fleet",
    pricing: "Pricing",
  },
  header: {
    brand: "MikCloud",
    signIn: "Sign in",
    signUp: "Free trial",
    langLabel: "FR",
    homeLink: "Back to home",
  },
  hero: {
    badge: "Hotspot management · HomeNet residential firewall · MikroTik cloud",
    title1: "Your WiFi,",
    titleAccent: "shielded",
    title2: "by the cloud.",
    subtitle:
      "Whether you run a public hotspot or shield your home with HomeNet, MikCloud brings vouchers, white-label captive portals, four firewall shields and full MikroTik fleet control together — one console, self-healed from the cloud.",
    ctaPrimary: "Protect my network",
    ctaSecondary: "Explore the platform",
    trialHint: "Free trial · no credit card — 60 days Hotspot · 30 days HomeNet",
    chips: ["4/4 protections active", "500 vouchers per batch", "Agent · 45 s check-in"],
  },
  marquee: [
    "Vouchers & access codes",
    "Quad9 DNS filtering",
    "WiFi anti-hacking",
    "Internet curfew",
    "HomeNet · Residential internet security",
    "VPN blocker",
    "QoS & ISP plan",
    "Reseller Sell Mode",
    "RouterOS updates",
    "White-label captive portal",
    "QR throwaway WiFi",
    "Telegram alerts",
    "Wave payments",
  ],
  powers: {
    eyebrow: "One console, three superpowers",
    title: "Everything you need to rule your network.",
    subtitle:
      "Hotspot management, cloud protection and fleet control: MikCloud gathers in a single interface everything your infrastructure demands — no Winbox, no server to maintain.",
    cards: [
      {
        tag: "Foundation",
        title: "Hotspot Management",
        desc: "MikCloud's historic core, all grown up: white-label captive portals, batch vouchers, per-plan quotas and throttling, live sessions.",
        items: [
          "100% white-label captive portal",
          "Voucher batches up to 500",
          "Quota, bandwidth & duration per plan",
          "Sell Mode & PIN-protected resellers",
        ],
      },
      {
        tag: "4 shields",
        title: "Cloud Protection",
        desc: "Four firewall protections deployed right on your routers, driven and self-healed from the cloud every six hours.",
        items: [
          "SafeWiFi — Quad9 / AdGuard DNS filtering",
          "Shield — admin ports locked down",
          "FamilyGuard — scheduled internet curfew",
          "AntiVPN — tunnels cut, WhatsApp intact",
        ],
      },
      {
        tag: "Fleet",
        title: "Fleet Control",
        desc: "Live telemetry, one-click RouterOS updates — router by router or the whole fleet at once — and continuously measured line quality.",
        items: [
          "RouterOS updates without Winbox",
          "CPU, memory & uptime telemetry",
          "Aggregate QoS & ISP plan",
          "IP pool doctor & self-healing",
        ],
      },
    ],
  },
  protection: {
    kicker: "Cloud protection",
    title: "The shield that watches while you sleep.",
    body: "Every protection is signature-checked and automatically repaired every six hours. Nothing to install, no Winbox needed: your existing rules are preserved, and the WiFi keeps working no matter what.",
    feats: [
      {
        title: "SafeWiFi — dangerous sites blocked",
        desc: "Viruses, scams and ads stopped at the DNS level (Quad9 or AdGuard Family) — DoH and IPv6 escape hatches are closed.",
      },
      {
        title: "Shield — WiFi anti-hacking",
        desc: "Your routers' admin ports and Windows file sharing become unreachable from the public WiFi.",
      },
      {
        title: "FamilyGuard — internet curfew",
        desc: "All client internet is cut during the scheduled window, for instance 10 pm to 6 am.",
      },
      {
        title: "AntiVPN — tunnels cut",
        desc: "WireGuard, OpenVPN, IPsec and Tor blocked — WhatsApp calls stay intact.",
      },
    ],
    panel: {
      title: "Protection center",
      scoreLabel: "Protection score",
      scoreVerdict: "Well protected",
      stats: [
        { value: "4", label: "protections on" },
        { value: "6 h", label: "self-healing" },
        { value: "45 s", label: "agent check-in" },
      ],
      lines: [
        { name: "SafeWiFi · DNS filtering", state: "Active" },
        { name: "Shield · Anti-hacking", state: "Active" },
        { name: "FamilyGuard · Curfew", state: "10 pm → 6 am" },
        { name: "AntiVPN · VPN blocker", state: "Active" },
      ],
      repairLine: { name: "Cloud self-healing", state: "Checked 2 min ago" },
    },
  },
  hotspot: {
    kicker: "Hotspot management",
    title: "A captive portal you'll be proud of.",
    body: "Give your customers a smooth, polished connection — under your brand, with zero MikCloud mention. Duration, quota or multi-device vouchers: you keep total control. Your resellers? They sell straight from their phone: PIN-protected PWA, transferred stock, offline sales, end-of-day report.",
    audience: ["Hotels", "Cybercafés", "Bars", "Shops", "Campuses", "Restaurants"],
    audienceLabel: "Who is it for?",
    feats: [
      {
        title: "100% custom branding",
        desc: "Commercial mode with price grid and Wave payments, or hospitality mode with promos and social links.",
      },
      {
        title: "Vouchers & plans",
        desc: "Duration, data quota, bandwidth, 1 to 10 devices — and when exhausted: cut off or softly throttle.",
      },
      {
        title: "Live statistics",
        desc: "Hourly footfall, revenue, margin per plan, direct and reseller channels.",
      },
    ],
    panel: {
      title: "Hourly footfall",
      online: "Peak hour · 7 pm",
      peak: "24 h",
    },
  },
  fleet: {
    kicker: "Fleet & routers",
    title: "Your MikroTik fleet, without leaving the console.",
    body: "The MikCloud agent dials out from the router to the cloud: it works behind CGNAT, Orange or Starlink, with no public IP and no open port. Then every router is remote-controlled — down to fleet-wide RouterOS updates.",
    feats: [
      {
        title: "One-click RouterOS updates",
        desc: "Check the latest version and install it router by router — or every router found behind, in a single move.",
      },
      {
        title: "Continuous telemetry",
        desc: "CPU, memory, uptime, RouterOS version and line quality for each router, refreshed on every check-in.",
      },
      {
        title: "Alerts that never sleep",
        desc: "Router offline, low voucher stock, daily report: Telegram, WhatsApp or email.",
      },
    ],
    panel: {
      title: "Router fleet",
      rows: [
        { name: "Plateau Café", version: "7.24.3", state: "Up to date", upToDate: true },
        { name: "Ebony Hotel", version: "7.16.2 → 7.24.3", state: "Update ready", upToDate: false },
        { name: "Market Cyber", version: "7.24.3", state: "Up to date", upToDate: true },
      ],
      checkAll: "Check the whole fleet",
      updateAll: "Update the fleet",
    },
  },
  stats: [
    { value: 500, label: "vouchers per batch" },
    { value: 4, label: "firewall shields" },
    { value: 54, label: "African countries" },
    { value: 2, label: "modes — Hotspot & HomeNet" },
  ],
  pricing: {
    eyebrow: "Pricing",
    title: "Two modes, one cloud.",
    subtitle: "Run a public network or shield your home: MikCloud adapts. The trial is free in both cases, no commitment.",
    modesLabel: "Choose your mode",
    segments: [
      {
        id: "hotspot",
        label: "Hotspot",
        hint: "Hotels, cybercafés, bars, shops, campuses…: vouchers, captive portal and access sales through your resellers.",
        audience: ["Hotels", "Cybercafés", "Bars", "Shops", "Campuses", "Restaurants"],
        plans: [
      {
        name: "Discovery",
        price: "0",
        period: "FCFA · 60 days",
        tagline: "To discover MikCloud risk-free",
        cta: "Start for free",
        highlight: false,
        features: [
          "1 router · every feature",
          "4 protections included",
          "Sell Mode & resellers",
          "No credit card",
        ],
      },
      {
        name: "Hotspot Yearly",
        price: "25,000",
        period: "FCFA / year",
        tagline: "All your routers, one single price",
        cta: "Go yearly",
        highlight: true,
        badge: "Most popular",
        features: [
          "Unlimited routers",
          "4 protections across the fleet",
          "Fleet RouterOS updates",
          "Telegram & WhatsApp alerts",
          "Priority support",
        ],
      },
      {
        name: "Hotspot Monthly",
        price: "2,500",
        period: "FCFA / month / router",
        tagline: "Pay as you grow",
        cta: "Choose monthly",
        highlight: false,
        features: [
          "Per active router",
          "No commitment",
          "Cancel anytime",
          "Every feature included",
        ],
      },
        ],
      },
      {
        id: "homenet",
        label: "HomeNet",
        hint: "Residential internet security: cloud firewall, DNS filtering, family curfew and device control for your household.",
        plans: [
          {
            name: "HomeNet Trial",
            price: "0",
            period: "FCFA · 30 days",
            tagline: "To protect your family risk-free",
            cta: "Start for free",
            highlight: false,
            features: [
              "1 router · every feature",
              "4 protections included",
              "Device control & dinner pause",
              "No credit card",
            ],
          },
          {
            name: "HomeNet Yearly",
            price: "12,000",
            period: "FCFA / year",
            tagline: "Your home shielded all year",
            cta: "Go yearly",
            highlight: true,
            badge: "Most popular",
            features: [
              "Unlimited routers — the whole family",
              "4 protections across the fleet",
              "Automatic RouterOS updates",
              "Telegram & WhatsApp alerts",
              "Priority support",
            ],
          },
          {
            name: "HomeNet Monthly",
            price: "1,250",
            period: "FCFA / month / router",
            tagline: "Protect your home, no commitment",
            cta: "Choose monthly",
            highlight: false,
            features: [
              "Per active router",
              "No commitment",
              "Cancel anytime",
              "Every feature included",
            ],
          },
        ],
      },
    ],
    currencyNote:
      "Payment fees passed through the list price: card +6%, Wave −3% (mobile money discount). Free trial: 60 days in Hotspot mode, 30 days in HomeNet mode.",
  },
  chat: {
    fabLabel: "Chat with the assistant",
    openLabel: "Open the conversation",
    closeLabel: "Close the conversation",
    tease: "Got a question? Ask away!",
    title: "MikCloud Assistant",
    statusBot: "Assistant · instant answers",
    statusHuman: "An advisor is replying to you",
    statusClosed: "Conversation closed",
    placeholder: "Type your message…",
    send: "Send",
    humanBtn: "Talk to a human",
    newConv: "New conversation",
    handoffNote: "Handed over to the MikCloud team — your next messages go straight to the advisor.",
    closedNote: "This conversation is over. See you soon on MikCloud!",
    suggestions: [
      "Hotspot or HomeNet?",
      "What are the prices?",
      "Which router do I need?",
      "Free trial?",
    ],
    agentBadge: "Support",
    inputLabel: "Your message",
    sendError: "Sending failed — try again",
  },
  finalCta: {
    kicker: "Ready to level up?",
    title: "Shield your WiFi in under 5 minutes.",
    subtitle:
      "Create your account, paste the agent script on your router, switch on your protections. No credit card, no server to maintain.",
    primary: "Create my free account",
    secondary: "Sign in",
  },
  footer: {
    tagline:
      "The cloud that protects: hotspot, firewall and MikroTik control united in a single tool.",
    columns: [
      {
        title: "Product",
        links: [
          { label: "Superpowers", href: "#pouvoirs" },
          { label: "Cloud protection", href: "#protection" },
          { label: "Hotspot", href: "#hotspot" },
          { label: "Router fleet", href: "#parc" },
          { label: "Pricing", href: "#tarifs" },
        ],
      },
      {
        title: "Console",
        links: [
          { label: "Sign in", href: "/login" },
          { label: "Sell Mode", href: "/sell" },
          { label: "Throwaway WiFi login", href: "/wifi" },
        ],
      },
      {
        title: "Legal",
        links: [{ label: "Privacy policy", href: "/legal/confidentialite" }],
      },
    ],
    legal: "Privacy policy",
  },
};

export const landingCopy: Record<Lang, LandingCopy> = { fr, en };
