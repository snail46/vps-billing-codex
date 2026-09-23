FROM golang:1.25-alpine AS build

WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bootstrap-admin ./cmd/bootstrap-admin \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/runman-gateway ./cmd/runman-gateway \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/issue-agent-token ./cmd/issue-agent-token

FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S app -G app
COPY --from=build /out/ /app/
USER app
EXPOSE 8080 9090
ENTRYPOINT ["/app/server"]
