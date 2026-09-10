# MikCloud Hotspot API — image Docker
# build : docker build -t mikcloud-api .
# run   : docker run -p 4000:4000 -v mikcloud-data:/app/data mikcloud-api
FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server .

FROM alpine:3.20
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
