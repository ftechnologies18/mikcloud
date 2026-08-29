# SpotCloud Hotspot API — image Docker
# build : docker build -t spotcloud-api .
# run   : docker run -p 4000:4000 -v spotcloud-data:/app/data spotcloud-api
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server .

FROM alpine:3.20
RUN adduser -D -u 10001 spotcloud
WORKDIR /app
COPY --from=builder /app/server /app/server
RUN mkdir -p /app/data && chown -R spotcloud:spotcloud /app
USER spotcloud
ENV PORT=4000 DATA_DIR=/app/data
EXPOSE 4000
VOLUME ["/app/data"]
ENTRYPOINT ["/app/server"]
