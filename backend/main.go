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
	"mikcloud/hotspot-api/internal/store"
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
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "mikcloud-dev-secret" // secret de dev ; en prod, définir JWT_SECRET
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

	// Arrêt propre (SIGTERM Render / Ctrl+C) : flush final vers Neon avant exit.
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGTERM, os.Interrupt)
		<-sig
		log.Printf("arrêt demandé — flush final des données…")
		st.Close()
		os.Exit(0)
	}()

	// Moniteur de surveillance (goroutine) : auto-mark offline des routeurs
	// agents (3 × 45 s sans check-in), alertes stock de vouchers bas et
	// rapport journalier — via les canaux configurés (Telegram/WhatsApp/Email).
	monitor := notify.NewService(st)
	go monitor.Run()

	handler := logRequests(corsMiddleware(authRateLimit(api.New(st, jwtSecret).Handler())))
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("MikCloud Hotspot API en écoute sur le port %s (données : %s)", port, dataDir)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("serveur : %v", err)
	}
}

// corsMiddleware — CORS restreint : ALLOWED_ORIGIN liste d'origines autorisées
// (ex. https://mikcloud.vercel.app,https://mikcloud-ftci.vercel.app). Vide ou *
// → ouvert (dev local). L'origine demandée est réfléchie si elle est autorisée.
func corsMiddleware(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range strings.Split(os.Getenv("ALLOWED_ORIGIN"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = true
		}
	}
	open := len(allowed) == 0 // dev : aucune origine configurée
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

// authRateLimit — 12 requêtes/minute/IP sur /api/auth/* (anti brute-force).
// Derrière la passerelle (Render/Caddy), l'IP client vient de X-Forwarded-For.
func authRateLimit(next http.Handler) http.Handler {
	type bucket struct {
		count int
		reset time.Time
	}
	const limit = 12
	var mu sync.Mutex
	buckets := map[string]*bucket{}
	go func() { // purge des fenêtres expirées
		for range time.Tick(time.Minute) {
			mu.Lock()
			for ip, b := range buckets {
				if time.Now().After(b.reset) {
					delete(buckets, ip)
				}
			}
			mu.Unlock()
		}
	}()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/auth/") {
			ip := clientIP(r)
			mu.Lock()
			now := time.Now()
			b := buckets[ip]
			if b == nil || now.After(b.reset) {
				b = &bucket{reset: now.Add(time.Minute)}
				buckets[ip] = b
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
		// premier IP de la chaîne = client d'origine
		if i := strings.IndexByte(xff, ','); i > 0 {
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
