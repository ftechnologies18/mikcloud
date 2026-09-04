// MikCloud Hotspot API — service backend Go (port 4000 par défaut, stdlib uniquement).
package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
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

	handler := logRequests(securityHeaders(corsMiddleware(limitBody(authRateLimit(api.New(st, jwtSecret).WithVitals(vitals).Handler())))))
	// Sécurité P1 #12 — timeouts HTTP complets. ReadHeaderTimeout seul laissait
	// des connexions en lecture/écriture illimitées : un client lent (ou hostile)
	// pouvait maintenir indéfiniment des goroutines et sockets (slowloris,
	// épuisement de ressources). Valeurs généreuses : aucun impact sur les flux
	// réels (agents 45 s, imports CSV, exports) — uniquement les connexions
	// pathologiquement lentes sont coupées. Aucun WebSocket : l'agent est en
	// HTTP-poll, le tableau de bord en polling.
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
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
		switch {
		case open:
			w.Header().Set("Access-Control-Allow-Origin", "*")
		case origin != "" && allowed[origin]:
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
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
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		}
		if r.ContentLength > maxBodyBytes {
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
//     buckets par IP.
//
// Les routes /agent/* (poll 45 s des routeurs, cadence fixe) et le healthcheck
// restent hors périmètre.
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
		case strings.HasPrefix(path, "/api/"):
			// Sécurité S1-A2 — limite globale par IP sur le reste de l'API.
			return "api", 120
		}
		return "", 0
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sécurité S1 (suivi A2) — plafond GLOBAL par instance : certaines
		// plates-formes relais transmettent le XFF du client (sondes
		// production : 15 XFF forgés → 15 buckets distincts), donc un
		// attaquant délibéré peut forger des IP pour échapper aux buckets par
		// IP. Ce compteur unique, insensible à toute usurpation d'en-tête,
		// borne le débit total admis par l'instance — la rotation d'IP ne le
		// contourne pas. 900/min ≈ 15 req/s soutenues, très au-dessus du
		// trafic légitime agrégé (consoles en polling, exports) — uniquement
		// les floods sont coupés.
		globalOK := true
		mu.Lock()
		if strings.HasPrefix(r.URL.Path, "/api/") {
			now := time.Now()
			gb := buckets["global|*"]
			if gb == nil || now.After(gb.reset) {
				gb = &bucket{reset: now.Add(time.Minute)}
				buckets["global|*"] = gb
			}
			gb.count++
			if gb.count > 900 {
				globalOK = false
			}
		}
		mu.Unlock()
		if !globalOK {
			w.Header().Set("Retry-After", "60")
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"Trop de requêtes — réessayez dans une minute"}`))
			return
		}
		if scope, limit := scopeFor(r.URL.Path); scope != "" {
			key := scope + "|" + clientIP(r)
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
			if !ok {
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
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}
