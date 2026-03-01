# ---- build stage ----
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /runner .

# ---- runtime stage ----
FROM debian:bookworm-slim

# Install Chromium and all required sandbox/media dependencies.
RUN apt-get update && apt-get install -y --no-install-recommends \
    chromium \
    chromium-sandbox \
    fonts-liberation \
    libasound2 \
    libatk-bridge2.0-0 \
    libatk1.0-0 \
    libcups2 \
    libdbus-1-3 \
    libdrm2 \
    libgbm1 \
    libgtk-3-0 \
    libnspr4 \
    libnss3 \
    libx11-6 \
    libxcomposite1 \
    libxdamage1 \
    libxext6 \
    libxfixes3 \
    libxrandr2 \
    xdg-utils \
    ca-certificates \
  && rm -rf /var/lib/apt/lists/*

# Create a non-root user; Chromium requires it when --no-sandbox is set.
RUN groupadd -r runner && useradd -r -g runner -G audio,video runner \
    && mkdir -p /home/runner && chown -R runner:runner /home/runner

WORKDIR /app
COPY --from=builder /runner .

USER runner
EXPOSE 8080

ENTRYPOINT ["./runner"]
