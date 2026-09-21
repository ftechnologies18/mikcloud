# MikCloud Hotspot API — image Docker
# build : docker build -t mikcloud-api .
# run   : docker run -p 4000:4000 -v mikcloud-data:/app/data mikcloud-api
FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server .

FROM alpine:3.20
# N°75 — ca-certificates : prérequis du sslmode=verify-full vers Neon (racines
# publiques pour la validation de la chaîne de certification). Sans elles, le
# driver pgx refuse la connexion TLS stricte → la synchro casse au démarrage.
# N°166 — la racine Supabase s'ajoute au magasin système : le pooler
# *.pooler.supabase.com sert une chaîne signée par la PKI PRIVÉE
# « Supabase Root 2021 CA » (racine auto-signée, absente des magasins
# publics) — sans elle, verify-full vers Supabase échoue en
# « x509: certificate signed by unknown authority » (reproduit le 20/09/2025).
# Certificat PUBLIC (empreinte SHA-256 80:70:25:AD:…:E6:CA:FA, valable
# jusqu'au 26/04/2031) — cf. docs/RUNBOOK-POSTGRES.md §12.
RUN apk add --no-cache ca-certificates
COPY certs/supabase-prod-ca-2021.crt /usr/local/share/ca-certificates/
RUN update-ca-certificates
RUN adduser -D -u 10001 mikcloud
WORKDIR /app
COPY --from=builder /app/server /app/server
RUN mkdir -p /app/data && chown -R mikcloud:mikcloud /app
USER mikcloud
# N°74 — GOMEMLIMIT : le runtime Go ne connaît pas la limite du conteneur
# (512 Mo sur Render free) : le GC attend ses propres seuils (≈2× le tas
# vivant) avant d'accélérer — trop tard face à l'OOM-kill. 400 MiB de plafond
# SOFT (le process ne meurt pas s'il doit dépasser, le GC fait juste tout son
# possible en dessous) laisse la marge de ~100 Mo pour le runtime.
ENV PORT=4000 DATA_DIR=/app/data GOMEMLIMIT=400MiB
EXPOSE 4000
VOLUME ["/app/data"]
ENTRYPOINT ["/app/server"]
