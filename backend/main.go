// MikCloud Hotspot API — service backend Go (port 4000 par défaut, stdlib uniquement).
package main

import (
        "log"
        "net/http"
        "os"
        "os/signal"
        "strconv"
        "syscall"
        "time"

        "mikcloud/hotspot-api/internal/api"
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

        // Arrêt propre (SIGTERM Render / Ctrl+C) : flush final vers Neon avant exit.
        go func() {
                sig := make(chan os.Signal, 1)
                signal.Notify(sig, syscall.SIGTERM, os.Interrupt)
                <-sig
                log.Printf("arrêt demandé — flush final des données…")
                st.Close()
                os.Exit(0)
        }()

        handler := logRequests(corsMiddleware(api.New(st, jwtSecret).Handler()))
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

// corsMiddleware — CORS ouvert (déploiement Vercel -> Render).
func corsMiddleware(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.Header().Set("Access-Control-Allow-Origin", "*")
                w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
                w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
                if r.Method == http.MethodOptions {
                        w.WriteHeader(http.StatusNoContent)
                        return
                }
                next.ServeHTTP(w, r)
        })
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
