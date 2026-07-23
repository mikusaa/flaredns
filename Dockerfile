# syntax=docker/dockerfile:1.7
FROM node:22-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.25-alpine AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /src/frontend/dist/ /src/frontend-dist/
RUN rm -rf internal/server/web && mkdir -p internal/server/web && cp -R /src/frontend-dist/. internal/server/web/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/flaredns ./cmd/flaredns

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata su-exec && addgroup -S -g 10001 flaredns && adduser -S -D -H -u 10001 -G flaredns flaredns
COPY --from=backend /out/flaredns /usr/local/bin/flaredns
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh
ENV FLAREDNS_ADDR=:8080 FLAREDNS_DATA_DIR=/data
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["flaredns"]
