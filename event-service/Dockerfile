# syntax=docker/dockerfile:1
# Multi-stage build for the Event Service (Service B).
# Build context is the repository root (see docker-compose.yml).

# ---- Stage 1: compile a static Go binary ----
FROM golang:1.23-alpine AS build
WORKDIR /src

# Download dependencies first so this layer is cached between code changes.
COPY go.mod go.sum ./
RUN go mod download

COPY gen/ gen/
COPY internal/ internal/
COPY event-service/ event-service/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/event-service ./event-service/cmd

# ---- Stage 2: minimal runtime image (no shell, no compiler, non-root) ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/event-service /event-service
USER nonroot:nonroot
ENTRYPOINT ["/event-service"]
