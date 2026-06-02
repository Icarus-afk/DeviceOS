FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=0.1.0-dev
ARG COMMIT=unknown

RUN CGO_ENABLED=0 go build -ldflags "\
    -X github.com/lohtbrok/deviceos/internal/version.Version=${VERSION} \
    -X github.com/lohtbrok/deviceos/internal/version.Commit=${COMMIT} \
    -s -w" \
    -o /out/deviceos ./cmd/deviceos

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

RUN adduser -D -h /data deviceos

COPY --from=builder /out/deviceos /usr/local/bin/deviceos

WORKDIR /data

USER deviceos

EXPOSE 8080

VOLUME ["/data"]

ENTRYPOINT ["deviceos"]
CMD ["start", "--config", "/data/deviceos.yaml"]
