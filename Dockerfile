FROM nvidia/cuda:12.4.0-devel-ubuntu22.04 AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    golang-1.22 git ca-certificates && \
    ln -s /usr/lib/go-1.22/bin/go /usr/local/bin/go && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o nvidia-exporter .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/nvidia-exporter /usr/local/bin/
EXPOSE 8082
ENTRYPOINT ["nvidia-exporter"]
