// MikCloud Hotspot API — service backend Go (port 4000 par défaut, stdlib uniquement).
package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	// N°10 — fuseaux horaires IANA embarqués (Tenant.Timezone fiable même
	// sur une image minimale de conteneur sans /usr/share/zoneinfo).
	_ "time/tzdata"

	"mikcloud/hotspot-api/internal/api"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/notify"
	"mikcloud/hotspot-api/internal/secretbox"
	"mikcloud/hotspot-api/internal/store"
	"mikcloud/hotspot-api/internal/telemetry"
)

// defaultPort — port d'écoute par défaut (overridable via PORT, ex. Render).
const defaultPort = 4000

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = strconv.Itoa(defaultPort)
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	// Sécurité P0 — le secret JWT ne doit JAMAIS tomber sur la constante de
	// développement en production : quiconque connaît le repo peut alors forger
	// des jetons super-admin (console plateforme, impersonation, purge).
	// Production (DATABASE_URL définie) → refus de démarrer. Dev local →
	// fallback toléré avec avertissement bruyant.
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		if os.Getenv("DATABASE_URL") != "" {
			log.Fatalf("sécurité : JWT_SECRET est obligatoire en production (DATABASE_URL définie) — définissez-la puis redéployez")
		}
		jwtSecret = "mikcloud-dev-secret" // développement local uniquement
		log.Println("AVERTISSEMENT : JWT_SECRET absente — secret de développement utilisé (jamais en production)")
	}

	// Sécurité P0 #6 — chiffrement au repos des identifiants RouterOS. La
	// clé vient de CREDENTIALS_KEY (64 hex) ou, à défaut, est dérivée de
	// JWT_SECRET (HKDF, domaine distinct). Fail-fast en production si aucune
	// source — AVANT toute écriture du store.
	if err := secretbox.Init(jwtSecret); err != nil {
		log.Fatalf("sécurité : %v", err)
	}

	st, err := store.New(dataDir)
	if err != nil {
		log.Fatalf("initialisation du store impossible : %v", err)
	}

	// N — réparation parité limit-uptime au démarrage : les vouchers coupés
	// par le routeur à leur quota temps mais restés « utilisés » (déficit
	// d'échantillonnage du cumul cloud, cf. RepairTimeLimitParity) repassent
	// « expirés » dès la remontée du service — sans attendre une lecture.
	st.Lock()
	if n := model.RepairTimeLimitParity(st.Data()); n > 0 {
		log.Printf("réparation parité limit-uptime : %d voucher(s) alignés sur leur quota — statut « expiré »", n)
		st.Save()
	}
	st.Unlock()

	// B2 « Speed App UX » — collecte des Core Web Vitals (POST /api/vitals) :
	// ring mémoire + insertions Neon par lots asynchrones (best-effort, file
	// bornée — un incident Neon n'impacte jamais les requêtes API). Sans
	// DATABASE_URL (dev), collecte mémoire seule. Le démarrage n'attend
	// jamais Neon : schéma + recharge historique en tâche de fond (45 s).
	vitals := telemetry.NewCollector(os.Getenv("DATABASE_URL"))

	// Arrêt propre (SIGTERM Render / Ctrl+C) : flush final vers Neon avant exit.
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGTERM, os.Interrupt)
		<-sig
		log.Printf("arrêt demandé — flush final des données…")
		vitals.Close()
		st.Close()
		os.Exit(0)
	}()

	// Moniteur de surveillance (goroutine) : auto-mark offline des routeurs
	// agents (3 × 45 s sans check-in), alertes stock de vouchers bas et
	// rapport journalier — via les canaux configurés (Telegram/WhatsApp/Email).
	monitor := notify.NewService(st)
	go monitor.Run()

	// N°64 — balayage PÉRIODIQUE de rétention : purge des journaux (90 j),
	// expirations et nettoyages ne dépendent plus des seules visites console
	// (Tick paresseux) — un compte dormant est couvert aussi. Rattrapage au
	// démarrage puis passage horaire ; preuve d'audit : GET / (lastSweepAt)
	// + traces de purge dans le log service.
	engine := api.New(st, jwtSecret).WithVitals(vitals)
	go engine.RunRetentionSweepForever()

	handler := logRequests(securityHeaders(corsMiddleware(limitBody(authRateLimit(engine.Handler())))))
	// Sécurité P1 #12 — timeouts HTTP complets. ReadHeaderTimeout seul laissait
	// des connexions en lecture/écriture illimitées : un client lent (ou hostile)
	// pouvait maintenir indéfiniment des goroutines et sockets (slowloris,
	// épuisement de ressources). Valeurs généreuses : aucun impact sur les flux
	// réels (agents 45 s, imports CSV, exports) — uniquement les connexions
	// pathologiquement lentes sont coupées. Aucun WebSocket : l'agent est en
	// HTTP-poll, le tableau de bord en polling.
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           recoverMiddleware(handler), // N°74 — filet de panique en tête de chaîne
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 16, // 64 Kio d'en-têtes (largement au-dessus du nominal)
	}
	log.Printf("MikCloud Hotspot API en écoute sur le port %s (données : %s)", port, dataDir)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("serveur : %v", err)
	}
}

// recoverMiddleware — N°74 — filet de sécurité anti-panique EN TÊTE de chaîne.
//
// L'audit de robustesse a montré que TOUT le backend converge vers le verrou
// global du store : une panique survenant entre un Lock() et son Unlock()
// (déverrouillage manuel avec early-returns dans la quasi-totalité des
// handlers) laisse le mutex verrouillé À VIE — toutes les requêtes suivantes
// se bloquent, le health check Render échoue, le service meurt en crash-loop.
// net/http rattrapait certes déjà les paniques de handlers (connexion coupée,
// process vivant), mais la classe « mutex mort » restait fatale.
//
// Ce middleware convertit toute panique de la chaîne (middlewares + handlers)
// en 500 propre + trace complète dans le log service : les defer des handlers
// se déroulent à la remontée, donc un éventuel Unlock différé s'exécute —
// le verrou survit à la panique.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec) // abort de pipeline voulu par net/http — ne pas convertir
				}
				log.Printf("PANIQUE récupérée (%s %s) : %v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"Erreur interne — réessayez"}`))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware — CORS restreint : ALLOWED_ORIGIN liste d'origines autorisées
// (ex. https://mikcloud.vercel.app,https://mikcloud-ftci.vercel.app). L'origine
// demandée est réfléchie si elle est autorisée.
// Sécurité P1 #16 — fail-closed en production : sans ALLOWED_ORIGIN, AUCUN
// en-tête CORS n'est émis (le navigateur refuse les requêtes cross-origin).
// Avant ce correctif, l'absence de configuration reflétait toutes les origines
// (« * ») : retirer la variable sur l'hébergeur aurait ouvert silencieusement
// l'API à n'importe quel site. Le mode permissif reste réservé au
// développement local (pas de DATABASE_URL).
func corsMiddleware(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range strings.Split(os.Getenv("ALLOWED_ORIGIN"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = true
		}
	}
	inProd := os.Getenv("DATABASE_URL") != ""
	open := len(allowed) == 0 && !inProd // dev local uniquement
	if len(allowed) == 0 && inProd {
		log.Println("AVERTISSEMENT : ALLOWED_ORIGIN absente en production — CORS en échec par défaut, requêtes cross-origin refusées (définissez ALLOWED_ORIGIN avec l'URL du frontend)")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// N°35-c — portail captif : les endpoints publics /api/wifi/site/*
		// (hors /sites et /guests qui restent console) acceptent N'IMPORTE
		// quelle origine. La page login.html est servie par le routeur avec
		// une origine imprévisible (IP du routeur, hostname DNS, etc.) —
		// la liste ALLOWED_ORIGIN (qui contient l'URL du frontend Vercel)
		// ne la couvrirait jamais. Ces endpoints sont PUBLICS par design
		// (aucun cookie, aucun JWT, juste un rate-limit IP + les plafonds
		// métier DailyPerPhone/DailyCap) — l'ouverture CORS est cohérente
		// avec leur nature publique et n'expose aucun credential.
		publicWifiCORS := strings.HasPrefix(r.URL.Path, "/api/wifi/site/") &&
			!strings.HasSuffix(r.URL.Path, "/sites") &&
			!strings.HasSuffix(r.URL.Path, "/guests")
		// N°56 — track analytics du portail : même logique que le
		// portail WiFi — l'origine de la page (le routeur) est
		// imprévisible, l'endpoint est public par design (clé
		// publique + dédup + quotas), CORS ouverte à toute origine.
		publicPromoCORS := r.URL.Path == "/api/portal/track"
		switch {
		case open:
			w.Header().Set("Access-Control-Allow-Origin", "*")
		case origin != "" && allowed[origin]:
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		case origin != "" && publicWifiCORS:
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Add("Vary", "Origin")
		case origin != "" && publicPromoCORS:
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		// N°74 — le préflight OPTIONS est mis en cache par le navigateur : chaque
		// POST cross-origin du portail (track analytics, claim WiFi) était DOUBLÉ
		// par une requête OPTIONS non cachée — jusqu'à 12 requêtes sur 6 utiles
		// par page en mode hospitalité. 24 h : borne haute fetch spec (sans credentials).
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// securityHeaders — en-têtes de sécurité HTTP sur TOUTES les réponses
// (sécurité S1-A4 : aucun de ces en-têtes n'était émis auparavant) :
//   - X-Content-Type-Options: nosniff — interdit le reniflage MIME du corps ;
//   - Strict-Transport-Security — HTTPS imposé aux visites ultérieures du
//     navigateur (no-op inoffensif sur le HTTP local de développement) ;
//   - X-Frame-Options: DENY — aucun framing (la facture HTML /api/billing/
//     invoice est ouverte en nouvel onglet via window.open, jamais en iframe) ;
//   - Referrer-Policy: no-referrer — aucune fuite d'URL vers des tiers ;
//   - Cache-Control: no-store — aucune réponse (données métier authentifiées,
//     factures) ne doit être retenue par un cache d'intermédiaire.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// maxBodyBytes — plafond de taille des corps de requête (sécurité S1-A1).
// 2 Mio couvre très largement les payloads JSON réels (création de vouchers,
// actions bulk, modèles, équipe) ; seuls les webhooks conservent leur propre
// borne plus stricte (1 Mio).
const maxBodyBytes = 2 << 20

// limitBody — plafonne la taille des corps de requête (sécurité S1-A1).
// decodeBody lisait le corps JSON SANS limite : un client malveillant pouvait
// poster des gigaoctets sur n'importe quelle route (épuisement mémoire du
// service, inflation du store). Double barrière :
//  1. Content-Length déclaré au-delà du plafond → 413 immédiat, corps jamais lu ;
//  2. http.MaxBytesReader coupe la lecture au plafond pour les corps streamés
//     (chunked) — l'erreur de décodage en amont produit une 400 et net/http
//     referme la connexion (aucune accumulation mémoire possible).
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// N°74 — cohérence des bornes : l'upload média (handleMediaUpload)
		// accepte 2 Mio de données + 1 Mio de marge d'encodage multipart
		// (3 Mio au total) — le plafond GLOBAL de 2 Mio le contredisait :
		// une image légitime de ~1,9 Mio pouvait prendre un 413 de la barrière
		// externe avant d'atteindre la validation du handler. /api/media
		// reçoit la borne cohérente, le reste conserve 2 Mio.
		limit := int64(maxBodyBytes)
		if strings.HasPrefix(r.URL.Path, "/api/media") {
			limit = maxBodyBytes + (1 << 20)
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		if r.ContentLength > limit {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_, _ = w.Write([]byte(`{"error":"Corps de requête trop volumineux (limite 2 Mio)"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authRateLimit — anti brute-force et anti-abus par IP :
//   - /api/auth/*           : 12 requêtes/minute (login console, inscription) ;
//   - /api/reseller/login   : 5 requêtes/minute — le PIN revendeur (4-6
//     chiffres) forme un espace de recherche minuscule, il exige une limite
//     plus dure (sécurité P0 : cette route était hors limiteur) ;
//   - toute autre route /api/* : 120 requêtes/minute (sécurité S1-A2 —
//     l'ancien périmètre ne couvrait que l'authentification : génération de
//     vouchers, actions bulk, demandes de paiement et administration étaient
//     sans limite par IP) ;
//   - plafond GLOBAL par instance de 900 requêtes/minute sur /api/* (suivi
//     S1-A2) : insensible à l'usurpation de X-Forwarded-For, il borne le
//     débit total même si un attaquant forge des IP pour échapper aux
//     buckets par IP ;
//   - N°75 — /agent/* (protocole des routeurs, historiquement hors
//     périmètre) : par TOKEN d'agent (les sites derrière NAT partagent une
//     IP — la clé pertinente est le routeur, pas l'origine) + garde IP
//     transverse. Un check-in légitime = 1,33 req/min à 45 s (moins en veille
//     N°75) : cmd 60/min par token = ~45× la marge ; result 120/min (rafales
//     de rapports au check-in chargé) ; register 6/min par IP (installation
//     manuelle ponctuelle). La garde IP 300/min sur tout /agent/* borne la
//     mémoire du limiteur contre les floods à tokens aléatoires (un bucket
//     par token forgé, purgé au bout de la fenêtre — l'IP de la source, elle,
//     est coupée bien avant).
//
// Les routes /agent/* (poll 45 s des routeurs, cadence fixe) et le healthcheck
// restent hors périmètre du limiteur /api/*.
//
// Derrière la passerelle (Render/Caddy), l'IP client vient du PREMIER hop
// de X-Forwarded-For — celui posé par le proxy de confiance (cf. clientIP).
func authRateLimit(next http.Handler) http.Handler {
	type bucket struct {
		count int
		reset time.Time
	}
	var mu sync.Mutex
	buckets := map[string]*bucket{}
	go func() { // purge des fenêtres expirées
		for range time.Tick(time.Minute) {
			mu.Lock()
			for k, b := range buckets {
				if time.Now().After(b.reset) {
					delete(buckets, k)
				}
			}
			mu.Unlock()
		}
	}()
	// scopeFor — scope du limiteur pour la requête : nom + limite, ou vide
	// si la route n'est pas limitée.
	scopeFor := func(path string) (string, int) {
		switch {
		case strings.HasPrefix(path, "/api/auth/"):
			return "auth", 12
		case path == "/api/reseller/login":
			return "reseller", 5
		case strings.HasPrefix(path, "/api/join/"):
			// N°27 — formulaire public d'inscription : borne serrée (bots).
			return "join", 10
		// N°28 — WiFi jetable : émission publique de tickets (surface d'attaque
		// critique : un anonyme qui crée des vouchers) → limite DURE ; lectures
		// publiques (branding, statut) → limite confortable pour un smartphone.
		case strings.HasPrefix(path, "/api/wifi/site/") && strings.HasSuffix(path, "/claim"):
			return "wifi-claim", 6
		case strings.HasPrefix(path, "/api/wifi/site/"):
			return "wifi-read", 30
		case strings.HasPrefix(path, "/api/"):
			// Sécurité S1-A2 — limite globale par IP sur le reste de l'API.
			return "api", 120
		}
		return "", 0
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// allow — comptage d'un bucket de fenêtre (1 min) ; true si la requête
		// passe. Verrou court, jamais tenu pendant un handler.
		allow := func(key string, limit int) bool {
			mu.Lock()
			now := time.Now()
			b := buckets[key]
			if b == nil || now.After(b.reset) {
				b = &bucket{reset: now.Add(time.Minute)}
				buckets[key] = b
			}
			b.count++
			ok := b.count <= limit
			mu.Unlock()
			return ok
		}

		// N°75 — protocole agent : par token quand il est lisible (cmd : query
		// string), par IP sinon (result : le token voyage dans le corps POST,
		// illisible au niveau middleware sans consommer le body ; register :
		// installation ponctuelle). Réponse 429 en TEXTE (le contrat des
		// endpoints agent est textuel — le fetch RouterOS échoue proprement,
		// retenté au tick suivant).
		if strings.HasPrefix(r.URL.Path, "/agent/") {
			ip := clientIP(r)
			if !allow("agent-ip|"+ip, 600) {
				rejectAgent(w)
				return
			}
			switch {
			case strings.HasPrefix(r.URL.Path, "/agent/cmd"):
				if !allow("agent-cmd|"+r.URL.Query().Get("token"), 60) {
					rejectAgent(w)
					return
				}
			case strings.HasPrefix(r.URL.Path, "/agent/result"):
				if !allow("agent-result|"+ip, 600) {
					rejectAgent(w)
					return
				}
			case strings.HasPrefix(r.URL.Path, "/agent/register"):
				if !allow("agent-register|"+ip, 6) {
					rejectAgent(w)
					return
				}
			}
			next.ServeHTTP(w, r)
			return
		}

		// Sécurité S1 (suivi A2) — plafond GLOBAL par instance : certaines
		// plates-formes relais transmettent le XFF du client (sondes
		// production : 15 XFF forgés → 15 buckets distincts), donc un
		// attaquant délibéré peut forger des IP pour échapper aux buckets par
		// IP. Ce compteur unique, insensible à toute usurpation d'en-tête,
		// borne le débit total admis par l'instance — la rotation d'IP ne le
		// contourne pas. 900/min ≈ 15 req/s soutenues, très au-dessus du
		// trafic légitime agrégé (consoles en polling, exports) — uniquement
		// les floods sont coupés.
		if strings.HasPrefix(r.URL.Path, "/api/") && !allow("global|*", 900) {
			w.Header().Set("Retry-After", "60")
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"Trop de requêtes — réessayez dans une minute"}`))
			return
		}
		if scope, limit := scopeFor(r.URL.Path); scope != "" {
			if !allow(scope+"|"+clientIP(r), limit) {
				w.Header().Set("Retry-After", "60")
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"Trop de tentatives — réessayez dans une minute"}`))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// rejectAgent — 429 du protocole agent : corps texte (le contrat de /agent/cmd
// est textuel — un JSON d'erreur serait juste du bruit dans le log routeur).
func rejectAgent(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "60")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte("# mikcloud: trop de requetes - reessayez dans une minute\n"))
}

// clientIP — l'adresse client réelle (X-Forwarded-For derrière un reverse proxy,
// sinon l'host de RemoteAddr sans le port source).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Sécurité S1 (suivi A2) — on retient le PREMIER hop de la chaîne :
		// c'est celui ajouté par le proxy de confiance Render, qui écrit l'IP
		// réelle du client EN TÊTE de X-Forwarded-For (convention Render) puis
		// ajoute ses hops internes à la suite. Les sondes de production de la
		// vague S1 ont démontré que le DERNIER hop (ancienne règle) est le hop
		// interne Render — il tourne d'une requête à l'autre, ce qui
		// fragmentait les buckets du limiteur (jamais de 429, même à 125
		// requêtes/minute ; 33 requêtes passées sur un scope à 12). La
		// plate-forme écrase toute valeur forgée par le client avant nous :
		// le premier hop est l'IP ajoutée par le proxy de confiance (guidance
		// MDN : ne faire confiance qu'aux IP ajoutées par des proxys de
		// confiance).
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// statusRecorder — capture le code de statut pour le log.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}

// logRequests — log minimal : méthode, chemin, statut, durée.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s -> %d (%s)", r.Method, maskLogPath(r.URL.Path), rec.status, time.Since(start).Round(time.Millisecond))
	})
}

// maskLogPath — N°74 — le chemin /portal/{token}/fichier expose le token
// d'agent (32 car., identifiant secret du routeur) dans les logs du service
// (rétention 7 j, visibles opérateur). Même discipline que les préfixes de
// l'agent (agent.Preview) : le segment token est remplacé par «***».
func maskLogPath(p string) string {
	if !strings.HasPrefix(p, "/portal/") {
		return p
	}
	rest := strings.TrimPrefix(p, "/portal/")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return "/portal/***" + rest[i:]
	}
	return "/portal/***"
}
