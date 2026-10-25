FROM node:24.19.0-alpine3.23@sha256:244cc2b53f46f9e876304391d17682b0ddae9ac33491f4857e25e35a36ba7995 AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.27.1-alpine3.23@sha256:0908ac9b9319e09d7c238aabe914e0395c51d63c4e3d0ae8c554fda9158a5769 AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /src/frontend/dist ./internal/staticui/dist
ARG VERSION=development
RUN CGO_ENABLED=0 go build -tags production -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /support ./cmd/support

FROM alpine:3.23.2@sha256:865b95f46d98cf867a156fe4a135ad3fe50d2056aa3f25ed31662dff6da4eb62
ARG VERSION=development
LABEL org.opencontainers.image.title="Support Desk" org.opencontainers.image.version=$VERSION
RUN addgroup -S -g 65532 support && adduser -S -D -H -u 65532 -G support support
COPY --from=backend /support /support
USER 65532:65532
EXPOSE 8080 9090
ENTRYPOINT ["/support"]
CMD ["serve"]
