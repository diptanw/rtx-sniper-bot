FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o sniper ./cmd/sniper

FROM gcr.io/distroless/static:nonroot
WORKDIR /root/

COPY --from=builder /app/sniper .

ENTRYPOINT ["/root/sniper"]
