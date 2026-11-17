FROM node:24-alpine AS frontend
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY index.html tsconfig.json vite.config.ts ./
COPY src ./src
COPY public ./public
RUN --mount=type=secret,id=amap_env,target=/app/.env.local npm run build

FROM golang:1.26.11-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /rental-atlas ./cmd/server

FROM alpine:3.23
RUN apk add --no-cache ca-certificates && addgroup -S atlas && adduser -S atlas -G atlas
WORKDIR /app
COPY --from=backend /rental-atlas ./rental-atlas
COPY --from=frontend /app/dist ./dist
COPY data/regions.json ./data/regions.json
COPY data/maps ./data/maps
COPY data/images ./data/images
COPY data/metro ./data/metro
RUN mkdir -p /app/runtime /app/data/runtime && chown -R atlas:atlas /app
USER atlas
ENV ADDR=0.0.0.0:8080 DATA_DIR=/app/data ALLOW_PUBLISH=false
EXPOSE 8080
CMD ["./rental-atlas"]
