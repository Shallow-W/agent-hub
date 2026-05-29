FROM golang:1.24-alpine AS backend-builder
WORKDIR /app
COPY src/backend/go.mod src/backend/go.sum ./
RUN go mod download
COPY src/backend/ .
RUN CGO_ENABLED=0 go build -o /agenthub-server ./cmd/server/

FROM node:20-alpine AS frontend-builder
WORKDIR /app
COPY src/frontend/package.json src/frontend/package-lock.json ./
RUN npm ci
COPY src/frontend/ .
RUN npm run build

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=backend-builder /agenthub-server /app/server
COPY --from=frontend-builder /app/dist /app/dist
COPY src/backend/config/config.example.yaml /app/config/config.yaml
COPY src/backend/migrations /app/migrations

ENV GIN_MODE=release
EXPOSE 8080
CMD ["/app/server"]
