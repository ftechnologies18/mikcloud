// chatbot.go — N°127 — cerveau de l'assistant conversationnel de la vitrine.
//
// Le bot remplace la section « Questions fréquentes » du landing : chaque
// question du visiteur est normalisée (minuscules, accents retirés,
// ponctuation espacée) puis confrontée à une base d'intents — un intent
// expose des mots-clés et une réponse par langue. L'intent au meilleur
// score (nombre de mots-clés reconnus) gagne ; à égalité, l'ordre de la
// liste tranche (les intents les plus spécifiques d'abord).
//
// Honnêteté : les réponses ne mentionnent QUE des fonctionnalités et des
// prix réels du produit (catalogue serveur N°122/N°123 : Hotspot 2 500 F
// /mois/routeur ou 25 000 F/an illimité ; HomeNet 1 250 F/mois/routeur ou
// 12 000 F/an illimité ; essai 60 jours Hotspot / 30 jours HomeNet sans
// carte bancaire). Aucune métrique d'usage inventée.
//
// L'intent "human" est spécial : il ne produit pas de réponse — le handler
// (handlers_chat.go) y déclenche la transmission au support.
package api

import (
	"strings"
	"unicode"

	"mikcloud/hotspot-api/internal/model"
)

// chatIntent — une entrée de la base de connaissances.
type chatIntent struct {
	id       string
	keywords []string // minuscules sans accents ; mot entier, ou sous-chaîne (≥ 5 caractères)
	answerFr string
	answerEn string
}

// chatIntents — base de connaissances. ORDRE = priorité à égalité de score
// (les intents spécifiques avant les génériques ; "greeting"/"thanks" en
// derniers : ils ne gagnent que lorsqu'ils sont seuls).
var chatIntents = []chatIntent{
	{
		id: "human",
		keywords: []string{
			"humain", "conseiller", "operateur", "support", "commercial",
			"personne", "quelquun", "contact", "contacter", "appeler",
			"telephone", "quelqu un", "vrai", "directement",
			"human", "advisor", "someone", "somebody",
			"real person", "talk to", "speak to", "support team", "sales",
		},
		// Pas de réponse : le handler déclenche la transmission.
	},
	{
		id: "modes",
		keywords: []string{
			"difference", "hotspot", "homenet", "residentiel", "residence",
			"maison", "foyer", "famille", "mode", "modes",
			"which mode", "home network", "home internet",
		},
		answerFr: "Le mode Hotspot s'adresse aux réseaux publics que vous exploitez (cybercafé, maquis, boutique) : vouchers, portail captif, revendeurs. Le mode HomeNet, c'est la sécurité internet résidentiel : pare-feu cloud, filtrage DNS, couvre-feu et contrôle des appareils pour votre foyer. Même console MikCloud, mêmes protections — seuls les tarifs diffèrent (HomeNet paie deux fois moins cher).",
		answerEn: "Hotspot mode targets the public networks you operate (cybercafé, bar, shop): vouchers, captive portal, resellers. HomeNet is residential internet security: cloud firewall, DNS filtering, curfew and device control for your household. Same MikCloud console, same protections — only the prices differ (HomeNet pays half the Hotspot rate).",
	},
	{
		id: "pricing",
		keywords: []string{
			"prix", "tarif", "tarifs", "coute", "cout", "combien", "cher",
			"abonnement", "facture", "fcfa", "2500", "1250", "25000",
			"12000", "mensuel", "annuel", "annuelle", "illimite",
			"price", "pricing", "cost", "how much", "subscription", "fee",
			"monthly", "yearly", "annual", "unlimited", "plan",
		},
		answerFr: "Deux modes, deux grilles. Hotspot (réseaux publics) : 2 500 F/mois par routeur, sans engagement — ou 25 000 F/an pour tous vos routeurs (illimité). HomeNet (sécurité internet résidentiel) : 1 250 F/mois par routeur, ou 12 000 F/an routeurs illimités. Des frais de paiement s'appliquent selon le moyen choisi (carte +6 %, Wave −3 %). Vous voulez essayer avant ? L'essai est gratuit et sans carte bancaire.",
		answerEn: "Two modes, two price grids. Hotspot (public networks): 2,500 F/month per router, no commitment — or 25,000 F/year for all your routers (unlimited). HomeNet (residential internet security): 1,250 F/month per router, or 12,000 F/year with unlimited routers. Payment fees apply depending on the method (card +6%, Wave −3%). Want to try first? The trial is free and requires no credit card.",
	},
	{
		id: "trial",
		keywords: []string{
			"essai", "gratuit", "gratuite", "tester", "test", "duree",
			"combien de temps", "jours", "carte bancaire", "engagement",
			"periode", "free", "trial", "try", "how long", "days",
			"credit card", "without card",
		},
		answerFr: "L'essai est gratuit et sans carte bancaire : 60 jours en mode Hotspot, 30 jours en mode HomeNet — la durée est posée à l'inscription selon votre usage, et l'équipe support peut la prolonger depuis la plateforme. Aucun engagement : vous ne payez que si vous continuez après l'essai.",
		answerEn: "The trial is free and requires no credit card: 60 days in Hotspot mode, 30 days in HomeNet mode — the duration is set at sign-up based on your usage, and the support team can extend it from the platform. No commitment: you only pay if you keep going after the trial.",
	},
	{
		id: "router",
		keywords: []string{
			"routeur", "mikrotik", "routeros", "winbox", "hex", "chr",
			"compatible", "materiel", "cgnat", "starlink",
			"ip publique", "installer", "installation", "agent", "script",
			"configurer", "mise en place", "brancher",
			"router", "hardware", "setup", "install", "configure", "public ip",
		},
		answerFr: "MikCloud pilote les routeurs MikroTik sous RouterOS (hEX, RB, CHR…). L'agent sort du routeur vers le cloud : il fonctionne derrière CGNAT, Orange ou Starlink, sans IP publique ni port ouvert — un script à coller dans Winbox, environ 40 secondes. Toute la configuration est poussée automatiquement, rien à maintenir côté serveur.",
		answerEn: "MikCloud drives MikroTik routers running RouterOS (hEX, RB, CHR…). The agent dials out from the router to the cloud: it works behind CGNAT, Orange or Starlink, with no public IP and no open port — one script to paste into Winbox, about 40 seconds. The whole configuration is pushed automatically, nothing to maintain on the server side.",
	},
	{
		id: "protections",
		keywords: []string{
			"protection", "proteger", "pare-feu", "firewall", "dns",
			"filtrage", "virus", "malware", "phishing", "publicite",
			"pub", "couvre-feu", "antivpn", "vpn", "securite", "piratage",
			"pirate", "familyguard", "safewifi", "shield", "enfant",
			"parental", "ado", "protect", "security", "ads", "curfew",
			"parental control",
		},
		answerFr: "Quatre boucliers posés directement sur votre routeur : SafeWiFi (filtrage DNS qui bloque sites dangereux et publicités), Shield (ports d'administration et partage Windows inaccessibles depuis le WiFi public), FamilyGuard (couvre-feu internet, par exemple 22 h – 6 h) et AntiVPN (bloque les VPN qui contourneraient vos règles). Ils s'activent en un clic, se réparent tout seuls et vos clients ne voient rien : la navigation reste fluide, WhatsApp passe.",
		answerEn: "Four shields placed directly on your router: SafeWiFi (DNS filtering that blocks dangerous sites and ads), Shield (admin ports and Windows sharing unreachable from the public WiFi), FamilyGuard (internet curfew, e.g. 10 pm – 6 am) and AntiVPN (blocks VPNs that would bypass your rules). They switch on with one click, repair themselves, and your customers notice nothing: browsing stays smooth, WhatsApp keeps working.",
	},
	{
		id: "payment_stop",
		keywords: []string{
			"arrete", "arret", "arreter", "cesser", "stop", "stopper",
			"suspendu", "suspension", "grace", "expirer", "expire",
			"expiration", "impaye", "resilier", "resiliation", "annuler",
			"perds", "perdu", "perdre", "coupe",
			"stop paying", "unpaid", "cancel", "lapse",
		},
		answerFr: "Vous avez 30 jours de grâce après l'échéance, puis la console est suspendue. Vos routeurs continuent de servir vos clients et vos données sont conservées — un règlement suffit à rouvrir l'accès.",
		answerEn: "You get a 30-day grace period after the due date, then the console is suspended. Your routers keep serving your customers and your data is preserved — one payment reopens access.",
	},
	{
		id: "payments",
		keywords: []string{
			"paiement", "payer", "paye", "regler", "reglement", "versement",
			"moyen", "wave", "orange", "mtn", "momo", "moov", "mpesa",
			"airtel", "carte", "visa", "mastercard", "mobile money",
			"geniuspay", "stripe", "frais", "commission",
			"pay", "payment", "method", "fee",
		},
		answerFr: "Vous pouvez régler par mobile money (Wave, Orange Money, MTN MoMo, Moov, MPesa, Airtel Money…) ou par carte bancaire. Les frais dépendent du moyen : la carte ajoute environ 6 %, Wave retire environ 3 %. La formule reste la même — seul le montant final varie légèrement.",
		answerEn: "You can pay with mobile money (Wave, Orange Money, MTN MoMo, Moov, MPesa, Airtel Money…) or by credit card. Fees depend on the method: the card adds about 6%, Wave takes about 3% off. The plan stays the same — only the final amount slightly varies.",
	},
	{
		id: "vouchers",
		keywords: []string{
			"voucher", "vouchers", "code", "codes", "ticket", "tickets",
			"qr", "qrcode", "lot", "lots", "batch", "quota", "bride",
			"bridage", "limite", "portail", "captive", "identifiant",
			"identifiants", "wifi code",
		},
		answerFr: "Le cœur du mode Hotspot : des vouchers par lots (jusqu'à 500 par lot), avec quotas de temps et de données, bridage au forfait plutôt que coupure, portail captif 100 % à votre marque et QR codes prêts à imprimer. Les sessions se suivent en direct dans la console.",
		answerEn: "The heart of Hotspot mode: vouchers in batches (up to 500 per batch), with time and data quotas, throttling to your plan instead of cutting off, a captive portal 100% in your brand and print-ready QR codes. Sessions are tracked live in the console.",
	},
	{
		id: "resellers",
		keywords: []string{
			"revendeur", "revendeurs", "vendeur", "vendre", "vente",
			"boutique", "magasin", "mode vente", "pwa",
			"reseller", "resellers", "sell", "selling", "shop",
		},
		answerFr: "Oui, et même sans boutique : le Mode Vente tourne sur le téléphone de vos revendeurs (PWA protégée par PIN). Stock transféré, ventes même hors-ligne, reçu partageable sur WhatsApp et rapport de fin de journée.",
		answerEn: "Yes — even without a shop: Sell Mode runs on your resellers' phones (PWA protected by a PIN). Transferred stock, offline sales, receipts shareable on WhatsApp and an end-of-day report.",
	},
	{
		id: "countries",
		keywords: []string{
			"pays", "disponible", "afrique", "africain", "nigeria",
			"ghana", "senegal", "ivoire", "cameroun", "kenya", "mali",
			"burkina", "benin", "togo", "niger", "gabon", "congo", "rdc",
			"uemoa", "cemac", "international", "monde", "europe",
			"country", "countries", "available", "africa", "abroad",
		},
		answerFr: "MikCloud tourne dans le cloud : le service est utilisable partout où votre routeur a internet. Il est conçu pour le marché africain — multi mobile-money (Wave, Orange Money, MTN MoMo, Moov, MPesa, Airtel Money) et multi-devises (FCFA, NGN, GHS, KES…) — et la carte bancaire ouvre le reste du monde.",
		answerEn: "MikCloud runs in the cloud: the service works anywhere your router has internet. It is designed for the African market — multi mobile-money (Wave, Orange Money, MTN MoMo, Moov, MPesa, Airtel Money) and multi-currency (FCFA, NGN, GHS, KES…) — and the credit card opens up the rest of the world.",
	},
	{
		id: "greeting",
		keywords: []string{
			"bonjour", "bonsoir", "salut", "coucou", "hello", "hey",
			"good morning", "good evening", "good afternoon", "hi",
		},
		answerFr: "Bonjour ! Je suis l'assistant MikCloud. Je peux vous renseigner sur les modes Hotspot et HomeNet, les tarifs, l'essai gratuit, la compatibilité routeur ou les protections — touchez une suggestion ou posez votre question. Et si vous préférez un humain, demandez-le à tout moment.",
		answerEn: "Hello! I'm the MikCloud assistant. I can tell you about the Hotspot and HomeNet modes, pricing, the free trial, router compatibility or the protections — tap a suggestion or ask your question. And if you'd rather talk to a human, just ask at any time.",
	},
	{
		id: "thanks",
		keywords: []string{
			"merci", "thanks", "thank you", "super", "parfait", "genial",
			"cool", "top", "daccord", "nickel", "great", "perfect", "ok",
		},
		answerFr: "Avec plaisir ! Si vous avez une autre question — tarifs, essai, routeur, protections — je suis là. Et un conseiller humain peut reprendre la conversation à tout moment, sur simple demande.",
		answerEn: "My pleasure! If you have another question — pricing, trial, router, protections — I'm here. And a human advisor can take over the conversation at any time, just ask.",
	},
}

// chatIntentHuman — identifiant de l'intent spécial « transmission ».
const chatIntentHuman = "human"

// Messages du bot (une seule source, FR + EN).
const (
	chatFallbackFr = "Je n'ai pas de réponse certaine à cette question. Vous pouvez la reformuler, toucher une suggestion ci-dessous — ou demander un conseiller humain : je vous transmets en un clic."
	chatFallbackEn = "I don't have a certain answer to that question. You can rephrase it, tap a suggestion below — or ask for a human advisor: I'll transfer you in one tap."
	chatWelcomeFr  = "Bonjour et bienvenue sur MikCloud. Je suis l'assistant de la vitrine : modes Hotspot et HomeNet, tarifs, essai gratuit, compatibilité routeur, protections… Posez votre question ou touchez une suggestion. Vous pouvez aussi demander un humain à tout moment."
	chatWelcomeEn  = "Hello and welcome to MikCloud. I'm the showcase assistant: Hotspot and HomeNet modes, pricing, free trial, router compatibility, protections… Ask your question or tap a suggestion. You can also ask for a human at any time."
	chatHandoffFr  = "Je vous transmets à un membre de l'équipe MikCloud. Il reprendra cette conversation depuis la console de support — patientez un instant, vos prochains messages lui parviennent directement."
	chatHandoffEn  = "I'm handing you over to a member of the MikCloud team. They will pick up this conversation from the support console — hold on a moment, your next messages go straight to them."
	chatClosedFr   = "Conversation clôturée par le support MikCloud. Merci de votre visite — vous pouvez rouvrir une nouvelle conversation à tout moment."
	chatClosedEn   = "Conversation closed by MikCloud support. Thanks for visiting — you can start a new conversation at any time."
)

// chatDeaccent — remplace les caractères latins accentués courants par
// leur base ASCII. Le backend est stdlib pur (pas de golang.org/x/text) :
// ce mapping couvre les diacritiques latins usuels du français (marché
// primaire) — suffisant pour la reconnaissance de mots-clés.
var chatDeaccent = map[rune]rune{
	'à': 'a', 'á': 'a', 'â': 'a', 'ä': 'a', 'ã': 'a', 'å': 'a',
	'è': 'e', 'é': 'e', 'ê': 'e', 'ë': 'e',
	'ì': 'i', 'í': 'i', 'î': 'i', 'ï': 'i',
	'ò': 'o', 'ó': 'o', 'ô': 'o', 'ö': 'o', 'õ': 'o',
	'ù': 'u', 'ú': 'u', 'û': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n', 'ý': 'y', 'ÿ': 'y', 'œ': 'o', 'æ': 'a',
}

// normalizeChat — minuscules, diacritiques retirés, tout ce qui n'est pas
// lettre/chiffre remplacé par un espace, espaces compactés.
// Exemples : "Coûte combien ?" → "coute combien" ; "l'hôtel" → "l hotel".
func normalizeChat(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if rep, ok := chatDeaccent[r]; ok {
			r = rep
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// chatMatch — vrai si le mot-clé normalisé est reconnu dans le message
// normalisé : mot entier (borné par des espaces), ou sous-chaîne pour les
// mots-clés longs (≥ 5 caractères — tolère les collages apostrophe).
func chatMatch(msgNorm, kwNorm string) bool {
	if kwNorm == "" {
		return false
	}
	if len([]rune(kwNorm)) < 5 {
		return strings.Contains(" "+msgNorm+" ", " "+kwNorm+" ")
	}
	return strings.Contains(msgNorm, kwNorm)
}

// chatBotAnswer — réponse du bot pour un message de visiteur.
// Retourne (réponse, intentID, matched). intentID == "human" → le handler
// déclenche la transmission au support (la réponse est alors vide).
func chatBotAnswer(msg, lang string) (string, string, bool) {
	msgNorm := normalizeChat(msg)
	bestScore, bestIdx := 0, -1
	for i := range chatIntents {
		score := 0
		for _, kw := range chatIntents[i].keywords {
			if chatMatch(msgNorm, normalizeChat(kw)) {
				score++
			}
		}
		if score > bestScore {
			bestScore, bestIdx = score, i
		}
	}
	if bestIdx < 0 || bestScore < 1 {
		if lang == "en" {
			return chatFallbackEn, "", false
		}
		return chatFallbackFr, "", false
	}
	intent := chatIntents[bestIdx]
	if intent.id == chatIntentHuman {
		return "", chatIntentHuman, true
	}
	if lang == "en" {
		return intent.answerEn, intent.id, true
	}
	return intent.answerFr, intent.id, true
}

// chatWelcome — message d'accueil dans la langue de la conversation.
func chatWelcome(lang string) string {
	if lang == "en" {
		return chatWelcomeEn
	}
	return chatWelcomeFr
}

// chatHandoffMessage — message de transmission dans la langue de la conversation.
func chatHandoffMessage(lang string) string {
	if lang == "en" {
		return chatHandoffEn
	}
	return chatHandoffFr
}

// chatClosedMessage — message de clôture dans la langue de la conversation.
func chatClosedMessage(lang string) string {
	if lang == "en" {
		return chatClosedEn
	}
	return chatClosedFr
}

// newChatMessage — construit un message prêt à être appendé.
func newChatMessage(convID, sender, body, at string) model.ChatMessage {
	return model.ChatMessage{
		ID:             model.NewID("cmsg-"),
		ConversationID: convID,
		Sender:         sender,
		Body:           body,
		At:             at,
	}
}
