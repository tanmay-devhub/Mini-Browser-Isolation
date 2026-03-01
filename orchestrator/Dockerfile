# ---- build stage ----
FROM golang:1.24-alpine AS builder

WORKDIR /src

# Cache module downloads separately from source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /orchestrator .

# ---- runtime stage ----
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /orchestrator .

# Non-root user for security.
RUN addgroup -S app && adduser -S app -G app
USER app

EXPOSE 8090

ENTRYPOINT ["./orchestrator"]
