// Landing page 403Shield « Clay » — copie marketing bilingue FR/EN.
//
// Auto-contenue (à part du dictionnaire applicatif i18n) : la vitrine a un
// volume de copie marketing important qui n'a pas vocation à vivre dans le
// dictionnaire applicatif. La langue courante est lue via le store zustand
// (useHotspotStore.lang) — pas de rechargement au switch.
//
// Refonte Multipage Enterprise (façon Cloudflare / OVHcloud) :
// Refléte fidèlement toutes les capacités du backend Go :
// - Hotspot Cloud, Vouchers par lots (1-500), Mode Vente Revendeurs avec PIN, Portail captif hybride à votre marque.
// - Module Cybercafé (gestion des postes, import DHCP, attribution de temps, codes de session).
// - Serveur PPPoE (provisionnement auto, secrets abonnés, reconnexion live, renouvellement).
// - Protection Cloud (4 boucliers pare-feu : SafeWiFi Quad9/AdGuard, Shield anti-piratage, FamilyGuard couvre-feu, AntiVPN avec WhatsApp intact).
// - Tunnels WireGuard (VPN personnel IP fixe France dédiée à Marseille, Accès Pro pour routeurs/API/SSH, LAN étendu pour caméras/NAS).
// - Flotte & Supervision (Agent HTTP-poll, mise à jour RouterOS unitaire + flotte, QoS Manager, Docteur Pool IP).
// - Sécurité & Comptabilité (2FA TOTP, comptabilité multi-sites, bilans mensuels gelés, alertes Telegram/WhatsApp, paiements Wave & Stripe).

export type Lang = "fr" | "en";

export interface ProductPageCopy {
  meta: { title: string; description: string };
  hero: {
    badge: string;
    title1: string;
    titleAccent: string;
    title2: string;
    subtitle: string;
    ctaPrimary: string;
    ctaPrimaryHref: string;
    ctaSecondary: string;
    ctaSecondaryHref: string;
  };
  features: {
    eyebrow: string;
    title: string;
    cards: { title: string; desc: string }[];
  };
  note?: { title: string; body: string };
  cta: {
    kicker: string;
    title: string;
    body: string;
    primary: string;
    primaryHref: string;
    secondary: string;
    secondaryHref: string;
  };
}

export interface MegaMenuItem {
  title: string;
  desc: string;
  href: string;
  badge?: string;
}

export interface MegaMenuCategory {
  title: string;
  items: MegaMenuItem[];
}

export interface LandingCopy {
  rail: {
    home: string;
    powers: string;
    protection: string;
    hotspot: string;
    tunnels: string;
    fleet: string;
    pricing: string;
  };
  header: {
    brand: string;
    signIn: string;
    signUp: string;
    langLabel: string;
    homeLink: string;
    nav: {
      home: string;
      products: string;
      solutions: string;
      hotspot: string;
      vpn: string;
      tunnels: string;
      pricing: string;
      docs: string;
    };
    megaMenu: {
      products: MegaMenuCategory[];
      solutions: MegaMenuCategory[];
    };
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
    audience: string[];
    audienceLabel: string;
    feats: { title: string; desc: string }[];
    panel: {
      title: string;
      online: string;
      peak: string;
    };
  };
  tunnels: {
    kicker: string;
    title: string;
    body: string;
    feats: { title: string; desc: string }[];
    panel: {
      title: string;
      rows: { name: string; detail: string; state: string }[];
      console: string;
      shop: string;
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
  ispExtension: {
    kicker: string;
    title: string;
    subtitle: string;
    feats: { title: string; desc: string; tag: string }[];
  };
  stats: { value: number; label: string }[];
  pricing: {
    eyebrow: string;
    title: string;
    subtitle: string;
    modesLabel: string;
    segments: {
      id: "hotspot" | "tunnels";
      label: string;
      hint: string;
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
        href?: string;
      }[];
    }[];
    currencyNote: string;
  };
  chat: {
    fabLabel: string;
    openLabel: string;
    closeLabel: string;
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
    columns: { title: string; links: { label: string; href: string; external?: boolean }[] }[];
    legal: string;
  };
  pages: {
    hotspot: ProductPageCopy;
    vpn: ProductPageCopy;
    tunnels: ProductPageCopy;
  };
}

const fr: LandingCopy = {
  rail: {
    home: "Accueil",
    powers: "Super-pouvoirs",
    protection: "Protection",
    hotspot: "Hotspot",
    tunnels: "Tunnels WireGuard",
    fleet: "Parc routeurs",
    pricing: "Tarifs",
  },
  header: {
    brand: "403Shield",
    signIn: "Se connecter",
    signUp: "Essai gratuit",
    langLabel: "EN",
    homeLink: "Retour à l'accueil",
    nav: {
      home: "Accueil",
      products: "Produits",
      solutions: "Solutions",
      hotspot: "Hotspot",
      vpn: "VPN",
      tunnels: "Tunnels WireGuard",
      pricing: "Tarifs",
      docs: "Guides & Docs",
    },
    megaMenu: {
      products: [
        {
          title: "Accès & Rentabilité",
          items: [
            {
              title: "Hotspot WiFi Cloud",
              desc: "Portail captif à votre marque, vouchers en lot, forfaits débits/durée et gestion revendeurs.",
              href: "/hotspot",
              badge: "Paiement Wave",
            },
            {
              title: "Gestion Cybercafé",
              desc: "Attribution automatique de temps, postes de travail, import DHCP et tickets de session.",
              href: "/#hotspot",
            },
            {
              title: "Gestionnaire PPPoE ISP",
              desc: "Provisionnement automatique des abonnés, secrets d'accès et reconnexion live MikroTik.",
              href: "/#hotspot",
            },
          ],
        },
        {
          title: "Sécurité & Infra Cloud",
          items: [
            {
              title: "Protection Cloud (4 Boucliers)",
              desc: "SafeWiFi DNS Quad9, Shield anti-piratage WiFi, FamilyGuard couvre-feu et AntiVPN.",
              href: "/#protection",
              badge: "Auto-réparation",
            },
            {
              title: "Tunnels WireGuard & VPN",
              desc: "IP fixe France dédiée à Marseille, Accès Pro pour routeurs et LAN étendu caméras/NAS.",
              href: "/tunnels",
            },
            {
              title: "Pilotage de Flotte MikroTik",
              desc: "Supervision live, QoS Manager, Docteur Pool IP et mise à jour RouterOS en un clic.",
              href: "/#parc",
            },
          ],
        },
      ],
      solutions: [
        {
          title: "Secteurs & Usages",
          items: [
            {
              title: "Hôtellerie & Restauration",
              desc: "WiFi visiteurs élégant, promos intégrées, réseaux sociaux et portail hospitalité.",
              href: "/hotspot",
            },
            {
              title: "Cybercafés & Salles de Jeu",
              desc: "Gestion automatisée des postes, tickets de temps et coupure automatique.",
              href: "/hotspot",
            },
            {
              title: "PME & Fournisseurs d'Accès",
              desc: "Interconnexion multi-sites, supervision de parc et gestion d'abonnés PPPoE.",
              href: "/tunnels",
            },
            {
              title: "Travailleurs Nomades & Foyers",
              desc: "VPN personnel IP fixe France et mode HomeNet pour la protection internet résidentielle.",
              href: "/vpn",
            },
          ],
        },
      ],
    },
  },
  hero: {
    badge: "403Shield · Hotspot · VPN · Tunnels · PPPoE · Cybercafé",
    title1: "Votre réseau,",
    titleAccent: "sous bouclier.",
    title2: "",
    subtitle:
      "L'erreur 403, c'est pour les autres. Hotspot WiFi public rentable, quatre boucliers pare-feu dans le cloud, gestionnaire PPPoE & Cybercafé, et un tunnel WireGuard unifié pour tous vos usages (VPN IP fixe France, Accès Pro routeurs, caméras & NAS) : 403Shield réunit la plateforme réseau cloud de référence — powered by FTech CI.",
    ctaPrimary: "Protéger mon réseau",
    ctaSecondary: "Découvrir la suite",
    trialHint: "Essai gratuit · sans carte bancaire — 60 jours Hotspot · 30 jours HomeNet",
    chips: ["Erreur 403 — aux autres", "4/4 protections actives", "Tunnels WireGuard · IP fixe France"],
  },
  marquee: [
    "Vouchers & codes d'accès",
    "VPN WireGuard · IP fixe France",
    "Filtrage DNS Quad9",
    "Gestionnaire PPPoE & Abonnés",
    "Module Cybercafé & Postes",
    "Un seul tunnel, deux usages",
    "Anti-piratage WiFi",
    "Accès distant Pro — routeurs, caméras, NAS",
    "Couvre-feu internet",
    "Mode HomeNet · le foyer sur le même abonnement",
    "Bloque-VPN",
    "QoS & forfait FAI",
    "Mode Vente revendeurs",
    "Mise à jour RouterOS",
    "Portail captif à votre marque",
    "Paiement Wave & Mobile Money",
  ],
  powers: {
    eyebrow: "Une suite, quatre piliers d'excellence",
    title: "Tout ce qu'il faut pour régner sur votre réseau.",
    subtitle:
      "Hotspot rentable, protection cloud, pilotage de parc, gestion d'abonnés PPPoE/Cybercafé et tunnels WireGuard : 403Shield réunit en une seule suite tout ce que votre infrastructure réclame — sans Winbox obligatoire, sans serveur à maintenir.",
    cards: [
      {
        tag: "Fondation",
        title: "Hotspot & Cybercafé",
        desc: "Le pilier fondateur de la suite : portails captifs à votre marque, vouchers par lots (1-500), gestion des postes cybercafé, quotas et bridage au forfait, sessions en direct.",
        items: [
          "Portail captif 100 % à votre marque",
          "Vouchers par lots jusqu'à 500",
          "Module Cybercafé & import DHCP auto",
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
        tag: "Parc & ISP",
        title: "Pilotage & PPPoE ISP",
        desc: "Télémétrie live, mise à jour RouterOS en un clic, gestion des abonnés PPPoE, QoS Manager et qualité de ligne mesurée en continu.",
        items: [
          "Serveur PPPoE & secrets abonnés",
          "Mise à jour RouterOS de flotte",
          "Télémétrie CPU, mémoire, uptime",
          "Docteur Pool IP & auto-réparation",
        ],
      },
      {
        tag: "WireGuard",
        title: "Tunnels WireGuard",
        desc: "Un seul tunnel chiffré, deux usages : VPN personnel à IP fixe France et Accès Pro pour vos routeurs, caméras et NAS — une console, une boutique, un abonnement.",
        items: [
          "VPN personnel · IP fixe France dédiée",
          "Accès Pro — routeurs, caméras, NAS",
          "Console sans mot de passe · code e-mail",
          "Hébergé à Marseille · chiffré de bout en bout",
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
    kicker: "Gestion hotspot & cybercafé",
    title: "Un portail captif et un espace de vente dont vous serez fier.",
    body: "Offrez à vos clients une connexion fluide et soignée — à votre marque, sans aucune mention 403Shield. Vouchers à durée, quota, multi-appareils ou tickets cybercafé : vous gardez le contrôle total. Vos revendeurs vendent depuis leur téléphone : PWA protégée par PIN, stock transféré, ventes hors-ligne, rapport de journée.",
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
        title: "Espace Cybercafé intégré",
        desc: "Attribution directe de temps d'écran, import automatique des bails DHCP et libération de poste en 1 clic.",
      },
      {
        title: "Statistiques & Caisse en direct",
        desc: "Affluence horaire, revenus, marge par profil, journaux de caisse exportables CSV et suivi revendeurs.",
      },
    ],
    panel: {
      title: "Affluence horaire",
      online: "Heure de pointe · 19 h",
      peak: "24 h",
    },
  },
  tunnels: {
    kicker: "Tunnels WireGuard",
    title: "Un seul tunnel chiffré, tous vos réseaux.",
    body: "Le VPN 403Shield et l'Accès Pro ne font qu'un : c'est le même tunnel WireGuard, hébergé à Marseille et chiffré de bout en bout. Sur votre téléphone, il protège votre trafic et vous donne une IP fixe France ; branché sur vos réseaux, il devient Accès Pro — routeurs, caméras et NAS joignables en privé, même derrière CGNAT. Une console, une boutique, un abonnement : tout vit au même endroit.",
    feats: [
      {
        title: "VPN personnel — IP fixe France",
        desc: "Votre trafic sort par Marseille avec une adresse dédiée, rien qu'à vous. Activation en 30 secondes, configuration livrée par e-mail.",
      },
      {
        title: "Accès Pro — routeurs distants",
        desc: "Le même tunnel posé sur vos routeurs MikroTik supervisés : administration, API et SSH à distance, sans exposer une seule interface sur Internet.",
      },
      {
        title: "LAN étendu — caméras & NAS",
        desc: "L'Accès Pro étend le tunnel à votre réseau local : caméras, NAS et imprimantes restent joignables en privé, où que vous soyez.",
      },
      {
        title: "Une console, une boutique",
        desc: "Un code e-mail à 6 chiffres suffit : achats, renouvellement 1-clic et portefeuille d'appareils — VPN et Accès Pro au même endroit.",
      },
    ],
    panel: {
      title: "Tunnel WireGuard · Marseille",
      rows: [
        { name: "VPN personnel", detail: "IP fixe France dédiée", state: "Actif" },
        { name: "Accès Pro · Routeurs", detail: "Supervision distante", state: "Actif" },
        { name: "LAN étendu · Caméras & NAS", detail: "Extension privée", state: "Chiffré" },
      ],
      console: "Ouvrir la console",
      shop: "Voir la boutique",
    },
  },
  fleet: {
    kicker: "Parc & flotte",
    title: "Votre parc MikroTik, sans quitter la console.",
    body: "L'agent 403Shield sort du routeur vers le cloud : il fonctionne derrière CGNAT, Orange ou Starlink, sans IP publique ni port ouvert. Puis chaque routeur se pilote à distance — jusqu'à la mise à jour RouterOS de toute la flotte.",
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
  ispExtension: {
    kicker: "Extension Opérateurs & Fournisseurs d'Accès",
    title: "Gestion d'abonnés PPPoE & Multi-Sites à grande échelle.",
    subtitle: "Transformez vos routeurs MikroTik en nœuds de distribution ISP professionnels sans complexité d'infrastructure.",
    feats: [
      {
        tag: "PPPoE Server Auto",
        title: "Provisionnement d'abonnés PPPoE",
        desc: "Détection automatique des pools LAN et création de profils abonnés sur les serveurs PPPoE MikroTik via agent binaire.",
      },
      {
        tag: "Reconnexion Live",
        title: "Kick & Renouvellement instantané",
        desc: "Déconnectez ou renouvelez le forfait d'un abonné PPPoE en un clic depuis le dashboard d'administration.",
      },
      {
        tag: "Comptabilité Multi-Sites",
        title: "Rapports & Clôtures mensuelles gelées",
        desc: "Agrégez les revenus de plusieurs établissements ou nœuds réseau avec archivage certifié de journal de caisse.",
      },
      {
        tag: "Sécurité Rapprochée",
        title: "Authentification 2FA TOTP & Rôles Team",
        desc: "Accès sécurisé par double facteur TOTP et gestion d'équipe fine (Propriétaire, Gérant, Opérateur).",
      },
    ],
  },
  stats: [
    { value: 500, label: "vouchers par lot" },
    { value: 4, label: "boucliers pare-feu" },
    { value: 54, label: "pays africains visés" },
    { value: 2, label: "usages du tunnel — VPN & Accès Pro" },
  ],
  pricing: {
    eyebrow: "Tarifs",
    title: "Un abonnement, un tunnel.",
    subtitle:
      "Exploitez un réseau public ou protégez votre foyer : c'est le MÊME abonnement console — le mode HomeNet applique le tarif foyer sur la même formule. Les tunnels WireGuard se commandent sur la boutique dédiée, et l'essai reste offert.",
    modesLabel: "Console ou boutique ?",
    segments: [
      {
        id: "hotspot",
        label: "Hotspot · Cybercafé · HomeNet",
        hint: "Hôtels, cybercafés, maquis, boutiques, campus, FAI… : vouchers, portail captif, cybercafé et vente d'accès avec vos revendeurs. Un foyer ? C'est le même abonnement en mode HomeNet, au tarif réduit sur la même formule.",
        audience: ["Hôtels", "Cybercafés", "Maquis", "Boutiques", "Campus", "Restaurants", "Foyers"],
        plans: [
          {
            name: "Découverte",
            price: "0",
            period: "FCFA · 60 jours",
            tagline: "Pour découvrir 403Shield sans risque",
            cta: "Commencer gratuitement",
            highlight: false,
            features: [
              "1 routeur · toutes les fonctions",
              "4 protections incluses",
              "Mode Vente & revendeurs",
              "Mode foyer (HomeNet) : essai 30 jours",
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
              "Gestionnaire Cybercafé & PPPoE",
              "Mode foyer (HomeNet) : 12 000 F/an",
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
              "Mode foyer (HomeNet) : 1 250 F/mois/routeur",
              "Sans engagement",
              "Résiliable à tout moment",
              "Toutes les fonctions incluses",
            ],
          },
        ],
      },
      {
        id: "tunnels",
        label: "Tunnels · boutique",
        hint: "Un seul tunnel WireGuard, deux usages : VPN personnel à IP fixe France et Accès Pro pour vos routeurs, caméras et NAS. Achat sur la boutique dédiée, payable Wave ou carte.",
        plans: [
          {
            name: "VPN Mensuel",
            price: "1 500",
            period: "FCFA / mois",
            tagline: "Sans engagement, mois par mois",
            cta: "Acheter sur la boutique",
            highlight: false,
            href: "/checkout?plan=vpn-b2c-month",
            features: [
              "IP fixe France dédiée",
              "WireGuard — activation en 30 s",
              "Paiement Wave ou carte",
              "Configuration livrée par e-mail",
            ],
          },
          {
            name: "VPN Annuel",
            price: "15 000",
            period: "FCFA / an",
            tagline: "12 mois au prix de 10",
            badge: "2 mois offerts",
            highlight: true,
            href: "/checkout?plan=vpn-b2c-year",
            cta: "Acheter sur la boutique",
            features: [
              "IP fixe France dédiée",
              "WireGuard — activation en 30 s",
              "Renouvellement en 1 clic",
              "Console sans mot de passe",
            ],
          },
          {
            name: "Accès Pro",
            price: "3 500",
            period: "FCFA / mois · 35 000 / an",
            tagline: "Vos routeurs, caméras et NAS dans le tunnel",
            cta: "Souscrire l'Accès Pro",
            highlight: false,
            href: "https://console.403shield.ftci.fr/pro",
            features: [
              "Supervision distante des routeurs MikroTik",
              "LAN étendu — caméras & NAS",
              "Même tunnel WireGuard, même console",
              "Derrière CGNAT, sans port ouvert",
            ],
          },
        ],
      },
    ],
    currencyNote:
      "Frais de paiement répercutés sur le prix de liste : carte +6 %, Wave −3 % (remise mobile money). Un abonnement console, deux modes : essai 60 jours en Hotspot · 30 jours en HomeNet — le foyer paie le tarif réduit sur la même formule. Tunnels WireGuard (VPN & Accès Pro) : prix catalogue boutique, remise mobile money incluse.",
  },
  chat: {
    fabLabel: "Discuter avec l'assistant",
    openLabel: "Ouvrir la discussion",
    closeLabel: "Fermer la discussion",
    tease: "Une question ? Écrivez-la ici !",
    title: "Assistant 403Shield",
    statusBot: "Assistant · réponses instantanées",
    statusHuman: "Un conseiller vous répond",
    statusClosed: "Conversation clôturée",
    placeholder: "Écrivez votre message…",
    send: "Envoyer",
    humanBtn: "Parler à un humain",
    newConv: "Nouvelle conversation",
    handoffNote: "Transmis à l'équipe 403Shield — vos prochains messages partent directement au conseiller.",
    closedNote: "Cette conversation est terminée. À bientôt sur 403Shield !",
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
    title: "Blindez votre réseau en moins de 5 minutes.",
    subtitle:
      "Créez votre compte, collez le script agent sur votre routeur, activez vos protections. Besoin d'un VPN ? La boutique livre une configuration WireGuard en 30 secondes. Sans carte bancaire, sans serveur à maintenir.",
    primary: "Créer mon compte gratuit",
    secondary: "Se connecter",
  },
  footer: {
    tagline:
      "La suite sécurité réseau cloud : Hotspot, protection, cybercafé, PPPoE et tunnels WireGuard — VPN & Accès Pro — powered by FTech CI.",
    columns: [
      {
        title: "Produit",
        links: [
          { label: "Hotspot & Cybercafé", href: "/hotspot" },
          { label: "VPN — Tunnels WireGuard", href: "/vpn" },
          { label: "Accès Pro — Tunnels WireGuard", href: "/tunnels" },
          { label: "Protection cloud", href: "/#protection" },
          { label: "Tarifs", href: "/#tarifs" },
        ],
      },
      {
        title: "Console",
        links: [
          { label: "Se connecter", href: "/login" },
          { label: "Mode Vente", href: "/sell" },
          { label: "Connexion WiFi jetable", href: "/wifi" },
          { label: "Guides d'exploitation", href: "/docs/guides" },
        ],
      },
      {
        title: "Légal",
        links: [{ label: "Politique de confidentialité", href: "/legal/confidentialite" }],
      },
      {
        title: "Écosystème",
        links: [
          { label: "FTech CI", href: "https://ftci.fr", external: true },
          { label: "Console 403Shield", href: "https://console.403shield.ftci.fr", external: true },
          { label: "Boutique VPN", href: "/checkout" },
        ],
      },
    ],
    legal: "Politique de confidentialité",
  },

  pages: {
    hotspot: {
      meta: {
        title: "403Shield Hotspot & Cybercafé — WiFi public rentable et blindé",
        description:
          "Vouchers par lots de 500, module Cybercafé, portail captif 100 % à votre marque, Mode Vente et revendeurs, 4 boucliers pare-feu : exploitez un réseau rentable, piloté depuis le cloud.",
      },
      hero: {
        badge: "403Shield Hotspot & Cybercafé",
        title1: "Votre réseau public,",
        titleAccent: "rentable",
        title2: "et blindé.",
        subtitle:
          "Hôtels, cybercafés, maquis, boutiques, campus, FAI : vendez l'accès internet par vouchers ou tickets de temps, à votre marque, avec vos revendeurs — pendant que quatre boucliers pare-feu veillent sur le réseau. Essai gratuit 60 jours, sans carte bancaire.",
        ctaPrimary: "Démarrer l'essai gratuit",
        ctaPrimaryHref: "#signup",
        ctaSecondary: "Voir les tarifs",
        ctaSecondaryHref: "/#tarifs",
      },
      features: {
        eyebrow: "Ce que fait 403Shield Hotspot",
        title: "Le WiFi public et le cybercafé, transformés en business.",
        cards: [
          {
            title: "Portail captif 100 % à votre marque",
            desc: "Mode commercial avec grille tarifaire et paiement Wave, ou mode hospitalité avec promos et réseaux sociaux. Vos clients ne voient que vous.",
          },
          {
            title: "Vouchers & forfaits",
            desc: "Durée, quota de données, débit, 1 à 10 appareils — et à l'épuisement : couper ou brider en douceur. Jusqu'à 500 vouchers par lot.",
          },
          {
            title: "Module Cybercafé & Postes",
            desc: "Gestion automatisée des temps d'écran, import automatique DHCP et arrêt à distance des postes de travail.",
          },
          {
            title: "Mode Vente & revendeurs",
            desc: "Vos revendeurs vendent depuis leur téléphone : PWA protégée par PIN, stock transféré, ventes hors-ligne, rapport de journée.",
          },
        ],
      },
      cta: {
        kicker: "Prêt à lancer votre hotspot ou cybercafé ?",
        title: "60 jours d'essai, sans carte bancaire.",
        body: "Créez votre compte, collez le script agent sur votre routeur MikroTik, configurez votre premier forfait. Sans serveur à maintenir, sans Winbox.",
        primary: "Créer mon compte gratuit",
        primaryHref: "#signup",
        secondary: "Voir les tarifs",
        secondaryHref: "/#tarifs",
      },
    },
    vpn: {
      meta: {
        title: "403Shield VPN — IP fixe France sur WireGuard",
        description:
          "Un VPN WireGuard qui sort par notre serveur de Marseille avec une IP fixe France dédiée. Activation en 30 secondes, paiement Wave ou carte, géré depuis votre espace sans mot de passe.",
      },
      hero: {
        badge: "Tunnels WireGuard · VPN",
        title1: "Une IP fixe France.",
        titleAccent: "À vous seul.",
        title2: "",
        subtitle:
          "Votre trafic, chiffré de bout en bout par WireGuard, sort par notre serveur de Marseille avec une adresse IP dédiée. Activation en 30 secondes — sans toucher à un fichier de configuration. Et pour joindre vos routeurs, caméras ou NAS : l'Accès Pro étend ce même tunnel.",
        ctaPrimary: "Acheter mon VPN",
        ctaPrimaryHref: "/checkout",
        ctaSecondary: "Découvrir les tunnels",
        ctaSecondaryHref: "/tunnels",
      },
      features: {
        eyebrow: "Pourquoi 403Shield VPN",
        title: "Le VPN qui ne vous enferme pas.",
        cards: [
          {
            title: "IP fixe dédiée",
            desc: "Une adresse française rien qu'à vous : banques, services publics et plateformes qui refusent les IP partagées vous reconnaissent enfin.",
          },
          {
            title: "WireGuard, audité et léger",
            desc: "Le protocole moderne, plus rapide et plus léger que les VPN d'ancienne génération. Votre configuration reste votre propriété.",
          },
          {
            title: "Mobile Money & carte",
            desc: "Payez en Wave ou par carte bancaire, en FCFA. Le provisioning est automatique : la configuration part par e-mail dès l'encaissement confirmé.",
          },
          {
            title: "Sans mot de passe",
            desc: "Un code à 6 chiffres reçu par e-mail suffit : achats, renouvellement 1-clic et récupération de configuration depuis votre espace.",
          },
        ],
      },
      note: {
        title: "Un seul tunnel, deux usages.",
        body: "Mensuel sans engagement, ou annuel — 10 mois payés, 2 offerts. Prix en FCFA sur la boutique, remise mobile money incluse. Et quand vos réseaux doivent passer par le même chemin : l'Accès Pro étend ce même tunnel WireGuard à vos routeurs, caméras et NAS — même console, même boutique.",
      },
      cta: {
        kicker: "Prêt à chiffrer votre trafic ?",
        title: "Votre configuration part par e-mail en 30 secondes.",
        body: "Choisissez votre formule sur la boutique, payez en Wave ou par carte : la configuration WireGuard complète arrive par e-mail, prête à importer.",
        primary: "Acheter mon VPN",
        primaryHref: "/checkout",
        secondary: "J'ai déjà un VPN — mon espace",
        secondaryHref: "https://console.403shield.ftci.fr",
      },
    },
    tunnels: {
      meta: {
        title: "403Shield Tunnels WireGuard — VPN IP fixe France & Accès Pro : routeurs, caméras, NAS",
        description:
          "Un seul tunnel WireGuard chiffré, hébergé à Marseille : VPN personnel à IP fixe France et Accès Pro pour superviser routeurs MikroTik, caméras et NAS. Console sans mot de passe, payable en Mobile Money.",
      },
      hero: {
        badge: "Tunnels WireGuard",
        title1: "Un seul tunnel.",
        titleAccent: "Deux usages.",
        title2: "",
        subtitle:
          "VPN personnel à IP fixe France, Accès Pro pour vos routeurs, LAN étendu pour caméras et NAS : le même tunnel WireGuard chiffré de bout en bout, hébergé à Marseille — une console, une boutique, un abonnement. Achat, gestion et renouvellement au même endroit.",
        ctaPrimary: "Acheter mon VPN",
        ctaPrimaryHref: "/checkout",
        ctaSecondary: "Découvrir le VPN",
        ctaSecondaryHref: "/vpn",
      },
      features: {
        eyebrow: "Pourquoi 403Shield Tunnels",
        title: "Le pont privé entre vous et vos réseaux.",
        cards: [
          {
            title: "VPN personnel — le même tunnel",
            desc: "Votre trafic chiffré de bout en bout sort par Marseille avec une IP fixe France dédiée. C'est le même WireGuard, la même console — que vous protégiez vos déplacements ou supervisiez un parc.",
          },
          {
            title: "Accès Pro — routeurs",
            desc: "Supervision distante de vos routeurs MikroTik à travers le tunnel — sans exposer votre interface d'administration sur Internet.",
          },
          {
            title: "LAN étendu — caméras & NAS",
            desc: "L'Accès Pro étend le tunnel à votre LAN : caméras, NAS et autres équipements restent joignables en privé, jamais exposés.",
          },
          {
            title: "Une console, une boutique",
            desc: "Un code e-mail à 6 chiffres suffit : espace client, portefeuille d'appareils, renouvellement 1-clic — VPN et Accès Pro vivent au même endroit.",
          },
        ],
      },
      cta: {
        kicker: "Vos réseaux vous attendent",
        title: "Ouvrez votre tunnel en moins de 5 minutes.",
        body: "Code e-mail, boutique Wave/carte, configuration livrée par e-mail : le tunnel s'installe sans toucher à un fichier de configuration.",
        primary: "Ouvrir la console",
        primaryHref: "https://console.403shield.ftci.fr",
        secondary: "Voir la boutique VPN",
        secondaryHref: "/checkout",
      },
    },
  },
};

const en: LandingCopy = {
  rail: {
    home: "Home",
    powers: "Superpowers",
    protection: "Protection",
    hotspot: "Hotspot",
    tunnels: "WireGuard tunnels",
    fleet: "Router fleet",
    pricing: "Pricing",
  },
  header: {
    brand: "403Shield",
    signIn: "Sign in",
    signUp: "Free trial",
    langLabel: "FR",
    homeLink: "Back to home",
    nav: {
      home: "Home",
      products: "Products",
      solutions: "Solutions",
      hotspot: "Hotspot",
      vpn: "VPN",
      tunnels: "WireGuard tunnels",
      pricing: "Pricing",
      docs: "Guides & Docs",
    },
    megaMenu: {
      products: [
        {
          title: "Access & Profitability",
          items: [
            {
              title: "Hotspot WiFi Cloud",
              desc: "White-label captive portal, batch vouchers, speed/duration plans, and reseller management.",
              href: "/hotspot",
              badge: "Wave Payment",
            },
            {
              title: "Cybercafé Management",
              desc: "Automatic screen time allocation, workstations, DHCP import, and session tickets.",
              href: "/#hotspot",
            },
            {
              title: "ISP PPPoE Manager",
              desc: "Automated subscriber provisioning, access secrets, and live MikroTik reconnection.",
              href: "/#hotspot",
            },
          ],
        },
        {
          title: "Security & Cloud Infra",
          items: [
            {
              title: "Cloud Protection (4 Shields)",
              desc: "SafeWiFi Quad9 DNS, Shield WiFi anti-hacking, FamilyGuard curfew, and AntiVPN.",
              href: "/#protection",
              badge: "Self-healing",
            },
            {
              title: "WireGuard Tunnels & VPN",
              desc: "Dedicated France fixed IP in Marseille, Pro Access for routers, and extended LAN for CCTV/NAS.",
              href: "/tunnels",
            },
            {
              title: "MikroTik Fleet Control",
              desc: "Live telemetry, QoS Manager, IP Pool Doctor, and one-click RouterOS updates.",
              href: "/#parc",
            },
          ],
        },
      ],
      solutions: [
        {
          title: "Sectors & Use Cases",
          items: [
            {
              title: "Hospitality & Restaurants",
              desc: "Elegant visitor WiFi, integrated promos, social links, and hospitality portal.",
              href: "/hotspot",
            },
            {
              title: "Cybercafés & Gaming Arenas",
              desc: "Automated workstation management, time tickets, and automatic cut-off.",
              href: "/hotspot",
            },
            {
              title: "SMEs & ISPs",
              desc: "Multi-site interconnection, fleet supervision, and PPPoE subscriber management.",
              href: "/tunnels",
            },
            {
              title: "Nomads & Households",
              desc: "Personal fixed French IP VPN and HomeNet mode for residential internet security.",
              href: "/vpn",
            },
          ],
        },
      ],
    },
  },
  hero: {
    badge: "403Shield · Hotspot · VPN · Tunnels · PPPoE · Cybercafé",
    title1: "Your network,",
    titleAccent: "shielded.",
    title2: "",
    subtitle:
      "Error 403 is for the others. A revenue-ready public Hotspot, four cloud firewall shields, PPPoE & Cybercafé manager, and a unified WireGuard tunnel for all your uses (fixed French IP VPN, Pro Access for routers, cameras & NAS): 403Shield brings together the reference cloud network platform — powered by FTech CI.",
    ctaPrimary: "Protect my network",
    ctaSecondary: "Explore the suite",
    trialHint: "Free trial · no credit card — 60 days Hotspot · 30 days HomeNet",
    chips: ["Error 403 — for the others", "4/4 protections active", "WireGuard tunnels · fixed French IP"],
  },
  marquee: [
    "Vouchers & access codes",
    "WireGuard VPN · fixed French IP",
    "Quad9 DNS filtering",
    "PPPoE & Subscriber Manager",
    "Cybercafé Module & Stations",
    "One tunnel, two uses",
    "WiFi anti-hacking",
    "Pro remote access — routers, cameras, NAS",
    "Internet curfew",
    "HomeNet mode · your household, same subscription",
    "VPN blocker",
    "QoS & ISP plan",
    "Reseller Sell Mode",
    "RouterOS updates",
    "White-label captive portal",
    "Wave & Mobile Money payments",
  ],
  powers: {
    eyebrow: "One suite, four pillars of excellence",
    title: "Everything you need to rule your network.",
    subtitle:
      "A profitable Hotspot, cloud protection, fleet control, PPPoE/Cybercafé management and WireGuard tunnels: 403Shield gathers in a single suite everything your infrastructure demands — no mandatory Winbox, no server to maintain.",
    cards: [
      {
        tag: "Foundation",
        title: "Hotspot & Cybercafé",
        desc: "The suite's founding pillar: white-label captive portals, batch vouchers (1-500), cybercafé station management, per-plan quotas and throttling, live sessions.",
        items: [
          "100% white-label captive portal",
          "Voucher batches up to 500",
          "Cybercafé module & auto DHCP import",
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
        tag: "Fleet & ISP",
        title: "Fleet Control & PPPoE ISP",
        desc: "Live telemetry, one-click RouterOS updates, PPPoE subscriber management, QoS Manager and continuously measured line quality.",
        items: [
          "PPPoE server & subscriber secrets",
          "Fleet RouterOS updates",
          "CPU, memory & uptime telemetry",
          "IP pool doctor & self-healing",
        ],
      },
      {
        tag: "WireGuard",
        title: "WireGuard Tunnels",
        desc: "One encrypted tunnel, two uses: a personal VPN with a fixed French IP and Pro Access for your routers, cameras and NAS — one console, one shop, one subscription.",
        items: [
          "Personal VPN · dedicated fixed French IP",
          "Pro Access — routers, cameras, NAS",
          "Passwordless console · email code",
          "Hosted in Marseille · encrypted end-to-end",
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
    kicker: "Hotspot & Cybercafé management",
    title: "A captive portal and point of sale you'll be proud of.",
    body: "Give your customers a smooth, polished connection — under your brand, with zero 403Shield mention. Duration, quota, multi-device vouchers or cybercafé tickets: you keep total control. Your resellers? They sell straight from their phone: PIN-protected PWA, transferred stock, offline sales, end-of-day report.",
    audience: ["Hotels", "Cybercafés", "Bars", "Shops", "Campuses", "Restaurants"],
    audienceLabel: "Who is it for?",
    feats: [
      {
        title: "100% custom branding",
        desc: "Commercial mode with rate card and Wave payments, or hospitality mode with promos and social links.",
      },
      {
        title: "Vouchers & plans",
        desc: "Duration, data quota, bandwidth, 1 to 10 devices — and when exhausted: cut off or throttle gently.",
      },
      {
        title: "Integrated Cybercafé Space",
        desc: "Direct allocation of screen time, automatic DHCP leases import and 1-click station release.",
      },
      {
        title: "Live Statistics & Cash Control",
        desc: "Hourly footfall, revenue, margin per plan, CSV exportable cash registers and reseller tracking.",
      },
    ],
    panel: {
      title: "Hourly footfall",
      online: "Peak hour · 7 pm",
      peak: "24 h",
    },
  },
  tunnels: {
    kicker: "WireGuard tunnels",
    title: "One encrypted tunnel, every network of yours.",
    body: "The 403Shield VPN and Pro Access are one and the same: a single WireGuard tunnel, hosted in Marseille and encrypted end-to-end. On your phone, it shields your traffic and hands you a fixed French IP; wired to your networks, it becomes Pro Access — routers, cameras and NAS reachable privately, even behind CGNAT. One console, one shop, one subscription: everything lives in the same place.",
    feats: [
      {
        title: "Personal VPN — fixed French IP",
        desc: "Your traffic exits through Marseille with a dedicated address, yours alone. 30-second activation, configuration delivered by email.",
      },
      {
        title: "Pro Access — remote routers",
        desc: "The same tunnel laid onto your supervised MikroTik routers: administration, API and SSH from anywhere — without exposing a single interface to the Internet.",
      },
      {
        title: "Extended LAN — cameras & NAS",
        desc: "Pro Access extends the tunnel to your local network: cameras, NAS and printers stay privately reachable, wherever you are.",
      },
      {
        title: "One console, one shop",
        desc: "A 6-digit email code is enough: purchases, 1-click renewal and device wallet — VPN and Pro Access in the same place.",
      },
    ],
    panel: {
      title: "WireGuard tunnel · Marseille",
      rows: [
        { name: "Personal VPN", detail: "Dedicated fixed French IP", state: "Active" },
        { name: "Pro Access · Routers", detail: "Remote supervision", state: "Active" },
        { name: "Extended LAN · Cameras & NAS", detail: "Private extension", state: "Encrypted" },
      ],
      console: "Open the console",
      shop: "Visit the shop",
    },
  },
  fleet: {
    kicker: "Fleet & routers",
    title: "Your MikroTik fleet, without leaving the console.",
    body: "The 403Shield agent dials out from the router to the cloud: it works behind CGNAT, Orange or Starlink, with no public IP and no open port. Then every router is remote-controlled — down to fleet-wide RouterOS updates.",
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
  ispExtension: {
    kicker: "Carrier & ISP Extension",
    title: "Large Scale PPPoE Subscriber & Multi-Site Management.",
    subtitle: "Turn your MikroTik routers into professional ISP distribution nodes without infrastructure complexity.",
    feats: [
      {
        tag: "PPPoE Server Auto",
        title: "PPPoE Subscriber Provisioning",
        desc: "Automatic detection of LAN pools and creation of subscriber profiles on MikroTik PPPoE servers via binary agent.",
      },
      {
        tag: "Live Reconnect",
        title: "Instant Kick & Renewal",
        desc: "Disconnect or renew a PPPoE subscriber's plan in one click directly from the admin dashboard.",
      },
      {
        tag: "Multi-Site Accounting",
        title: "Reports & Frozen Monthly Closings",
        desc: "Aggregate revenue across multiple locations or network nodes with certified cash journal archiving.",
      },
      {
        tag: "Close-up Security",
        title: "2FA TOTP Auth & Team Roles",
        desc: "Secured access with two-factor TOTP authentication and fine-grained team management (Owner, Manager, Operator).",
      },
    ],
  },
  stats: [
    { value: 500, label: "vouchers per batch" },
    { value: 4, label: "firewall shields" },
    { value: 54, label: "African countries" },
    { value: 2, label: "uses of the tunnel — VPN & Pro Access" },
  ],
  pricing: {
    eyebrow: "Pricing",
    title: "One subscription, one tunnel.",
    subtitle:
      "Run a public network or shield your home: it's the SAME console subscription — HomeNet mode applies the household rate to the same plan. WireGuard tunnels are ordered on the dedicated shop, and the trial stays free.",
    modesLabel: "Console or shop?",
    segments: [
      {
        id: "hotspot",
        label: "Hotspot · Cybercafé · HomeNet",
        hint: "Hotels, cybercafés, bars, shops, campuses, ISPs…: vouchers, captive portal, cybercafé and access sales through your resellers. A household? It's the same subscription in HomeNet mode, at the reduced rate on the same plan.",
        audience: ["Hotels", "Cybercafés", "Bars", "Shops", "Campuses", "Restaurants", "Households"],
        plans: [
          {
            name: "Discovery",
            price: "0",
            period: "FCFA · 60 days",
            tagline: "To discover 403Shield risk-free",
            cta: "Start for free",
            highlight: false,
            features: [
              "1 router · every feature",
              "4 protections included",
              "Sell Mode & resellers",
              "HomeNet mode (household): 30-day trial",
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
              "Cybercafé & PPPoE manager",
              "HomeNet mode (household): 12,000 F/year",
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
              "HomeNet mode (household): 1,250 F/month/router",
              "No commitment",
              "Cancel anytime",
              "Every feature included",
            ],
          },
        ],
      },
      {
        id: "tunnels",
        label: "Tunnels · shop",
        hint: "One WireGuard tunnel, two uses: a personal VPN with a fixed French IP and Pro Access for your routers, cameras and NAS. Ordered on the dedicated shop, payable with Wave or card.",
        plans: [
          {
            name: "VPN Monthly",
            price: "1,500",
            period: "FCFA / month",
            tagline: "No commitment, month by month",
            cta: "Buy on the shop",
            highlight: false,
            href: "/checkout?plan=vpn-b2c-month",
            features: [
              "Dedicated fixed French IP",
              "WireGuard — 30 s activation",
              "Wave or card payment",
              "Configuration delivered by email",
            ],
          },
          {
            name: "VPN Yearly",
            price: "15,000",
            period: "FCFA / year",
            tagline: "12 months for the price of 10",
            badge: "2 months free",
            highlight: true,
            href: "/checkout?plan=vpn-b2c-year",
            cta: "Buy on the shop",
            features: [
              "Dedicated fixed French IP",
              "WireGuard — 30 s activation",
              "1-click renewal",
              "Passwordless console",
            ],
          },
          {
            name: "Pro Access",
            price: "3,500",
            period: "FCFA / month · 35,000 / year",
            tagline: "Your routers, cameras and NAS in the tunnel",
            cta: "Subscribe to Pro Access",
            highlight: false,
            href: "https://console.403shield.ftci.fr/pro",
            features: [
              "Remote supervision of MikroTik routers",
              "Extended LAN — cameras & NAS",
              "Same WireGuard tunnel, same console",
              "Behind CGNAT, no open port",
            ],
          },
        ],
      },
    ],
    currencyNote:
      "Payment fees passed through the list price: card +6%, Wave −3% (mobile money discount). One console subscription, two modes: 60-day trial in Hotspot · 30-day in HomeNet — the household pays the reduced rate on the same plan. WireGuard tunnels (VPN & Pro Access): shop catalogue prices, mobile money discount included.",
  },
  chat: {
    fabLabel: "Chat with the assistant",
    openLabel: "Open the conversation",
    closeLabel: "Close the conversation",
    tease: "Got a question? Ask away!",
    title: "403Shield Assistant",
    statusBot: "Assistant · instant answers",
    statusHuman: "An advisor is replying to you",
    statusClosed: "Conversation closed",
    placeholder: "Type your message…",
    send: "Send",
    humanBtn: "Talk to a human",
    newConv: "New conversation",
    handoffNote: "Forwarded to the 403Shield team — your next messages go straight to the advisor.",
    closedNote: "This conversation is over. See you soon on 403Shield!",
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
    title: "Shield your network in under 5 minutes.",
    subtitle:
      "Create your account, paste the agent script on your router, switch on your protections. Need a VPN? The shop delivers a WireGuard configuration within 30 seconds. No credit card, no server to maintain.",
    primary: "Create my free account",
    secondary: "Sign in",
  },
  footer: {
    tagline:
      "The cloud network-security suite: Hotspot, protection, cybercafé, PPPoE and WireGuard tunnels — VPN & Pro Access — powered by FTech CI.",
    columns: [
      {
        title: "Product",
        links: [
          { label: "Hotspot & Cybercafé", href: "/hotspot" },
          { label: "VPN — WireGuard tunnels", href: "/vpn" },
          { label: "Pro Access — WireGuard tunnels", href: "/tunnels" },
          { label: "Cloud protection", href: "/#protection" },
          { label: "Pricing", href: "/#tarifs" },
        ],
      },
      {
        title: "Console",
        links: [
          { label: "Sign in", href: "/login" },
          { label: "Sell Mode", href: "/sell" },
          { label: "Throwaway WiFi login", href: "/wifi" },
          { label: "Operations guides", href: "/docs/guides" },
        ],
      },
      {
        title: "Legal",
        links: [{ label: "Privacy policy", href: "/legal/confidentialite" }],
      },
      {
        title: "Ecosystem",
        links: [
          { label: "FTech CI", href: "https://ftci.fr", external: true },
          { label: "403Shield Console", href: "https://console.403shield.ftci.fr", external: true },
          { label: "VPN shop", href: "/checkout" },
        ],
      },
    ],
    legal: "Privacy policy",
  },

  pages: {
    hotspot: {
      meta: {
        title: "403Shield Hotspot & Cybercafé — revenue-ready, hardened public WiFi",
        description:
          "Batch vouchers up to 500, Cybercafé module, 100% white-label captive portal, Sell Mode with resellers, and 4 firewall shields: run a profitable network, driven from the cloud.",
      },
      hero: {
        badge: "403Shield Hotspot & Cybercafé",
        title1: "Your public network,",
        titleAccent: "profitable",
        title2: "and hardened.",
        subtitle:
          "Hotels, cybercafés, eateries, shops, campuses, ISPs: sell internet access with vouchers or time tickets, under your brand, through your resellers — while four firewall shields watch over the network. 60-day free trial, no credit card.",
        ctaPrimary: "Start the free trial",
        ctaPrimaryHref: "#signup",
        ctaSecondary: "See pricing",
        ctaSecondaryHref: "/#tarifs",
      },
      features: {
        eyebrow: "What 403Shield Hotspot does",
        title: "Public WiFi and Cybercafé, turned into a business.",
        cards: [
          {
            title: "100% white-label captive portal",
            desc: "Commercial mode with rate card and Wave payments, or hospitality mode with promos and social links. Your customers only see you.",
          },
          {
            title: "Vouchers & plans",
            desc: "Duration, data quota, bandwidth, 1 to 10 devices — and on depletion: cut off or throttle gently. Up to 500 vouchers per batch.",
          },
          {
            title: "Cybercafé Module & Stations",
            desc: "Automated screen time management, automatic DHCP import and remote workstation shutoff.",
          },
          {
            title: "Sell Mode & resellers",
            desc: "Your resellers sell from their phone: PIN-protected PWA, transferred stock, offline sales, end-of-day report.",
          },
        ],
      },
      cta: {
        kicker: "Ready to launch your hotspot or cybercafé?",
        title: "60 days free, no credit card.",
        body: "Create your account, paste the agent script onto your MikroTik router, configure your first plan. No server to maintain, no Winbox.",
        primary: "Create my free account",
        primaryHref: "#signup",
        secondary: "See pricing",
        secondaryHref: "/#tarifs",
      },
    },
    vpn: {
      meta: {
        title: "403Shield VPN — fixed French IP over WireGuard",
        description:
          "A WireGuard VPN exiting through our Marseille server with a dedicated fixed French IP. 30-second activation, Wave or card payment, managed from your passwordless account.",
      },
      hero: {
        badge: "WireGuard tunnels · VPN",
        title1: "A fixed French IP.",
        titleAccent: "Yours alone.",
        title2: "",
        subtitle:
          "Your traffic, end-to-end encrypted by WireGuard, exits through our Marseille server with a dedicated IP address. 30-second activation — without touching a single config file. And to reach your routers, cameras or NAS: Pro Access extends that very same tunnel.",
        ctaPrimary: "Buy my VPN",
        ctaPrimaryHref: "/checkout",
        ctaSecondary: "Explore tunnels",
        ctaSecondaryHref: "/tunnels",
      },
      features: {
        eyebrow: "Why 403Shield VPN",
        title: "The VPN that doesn't lock you in.",
        cards: [
          {
            title: "Dedicated fixed IP",
            desc: "A French address just for you: banks, public services and platforms that reject shared IPs finally recognize you.",
          },
          {
            title: "WireGuard, audited & lightweight",
            desc: "The modern protocol, faster and lighter than legacy VPNs. Your configuration remains your property.",
          },
          {
            title: "Mobile Money & card",
            desc: "Pay with Wave or bank card, in FCFA. Provisioning is automatic: your configuration is emailed as soon as payment is confirmed.",
          },
          {
            title: "No password",
            desc: "A 6-digit code received by email is enough: purchases, 1-click renewal and configuration recovery from your account.",
          },
        ],
      },
      note: {
        title: "One tunnel, two uses.",
        body: "Monthly with no commitment, or yearly — pay 10 months, get 2 offered. Prices in FCFA on the shop, mobile-money discount included. And when your networks need the same path: Pro Access extends this very WireGuard tunnel to your routers, cameras and NAS — same console, same shop.",
      },
      cta: {
        kicker: "Ready to encrypt your traffic?",
        title: "Your configuration is emailed within 30 seconds.",
        body: "Pick your plan on the shop, pay with Wave or card: the full WireGuard configuration arrives by email, ready to import.",
        primary: "Buy my VPN",
        primaryHref: "/checkout",
        secondary: "I already have a VPN — my account",
        secondaryHref: "https://console.403shield.ftci.fr",
      },
    },
    tunnels: {
      meta: {
        title: "403Shield WireGuard Tunnels — fixed-IP VPN & Pro Access: routers, cameras, NAS",
        description:
          "One encrypted WireGuard tunnel hosted in Marseille: a personal VPN with a fixed French IP and Pro Access to supervise MikroTik routers, cameras and NAS. Passwordless console, payable with Mobile Money.",
      },
      hero: {
        badge: "WireGuard tunnels",
        title1: "One single tunnel.",
        titleAccent: "Two uses.",
        title2: "",
        subtitle:
          "A personal VPN with a fixed French IP, Pro Access for your routers, an extended LAN for cameras and NAS: the same end-to-end encrypted WireGuard tunnel, hosted in Marseille — one console, one shop, one subscription. Purchase, management and renewal in one place.",
        ctaPrimary: "Buy my VPN",
        ctaPrimaryHref: "/checkout",
        ctaSecondary: "Explore the VPN",
        ctaSecondaryHref: "/vpn",
      },
      features: {
        eyebrow: "Why 403Shield Tunnels",
        title: "The private bridge between you and your networks.",
        cards: [
          {
            title: "Personal VPN — the same tunnel",
            desc: "Your end-to-end encrypted traffic exits through Marseille with a dedicated fixed French IP. It is the same WireGuard, the same console — whether you shield your travels or supervise a fleet.",
          },
          {
            title: "Pro Access — routers",
            desc: "Remote supervision of your MikroTik routers through the tunnel — without exposing your admin interface to the Internet.",
          },
          {
            title: "Extended LAN — cameras & NAS",
            desc: "Pro Access extends the tunnel to your LAN: cameras, NAS and other equipment stay privately reachable, never exposed.",
          },
          {
            title: "One console, one shop",
            desc: "A 6-digit email code is enough: client area, device wallet, 1-click renewal — the VPN and Pro Access live in the same place.",
          },
        ],
      },
      cta: {
        kicker: "Your networks are waiting",
        title: "Open your tunnel in under 5 minutes.",
        body: "Email code, Wave/card shop, configuration delivered by email: the tunnel installs itself without touching a config file.",
        primary: "Open the console",
        primaryHref: "https://console.403shield.ftci.fr",
        secondary: "Browse the VPN shop",
        secondaryHref: "/checkout",
      },
    },
  },
};

export const landingCopy: Record<Lang, LandingCopy> = { fr, en };
