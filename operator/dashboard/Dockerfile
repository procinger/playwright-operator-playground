FROM docker.io/golang:1.25.2-alpine3.22 AS builder

WORKDIR /

ENV USER=app
ENV UID=1001

RUN mkdir -p /root/.cache

RUN adduser \
    --disabled-password \
    --gecos "" \
    --home "/nonexistent" \
    --shell "/sbin/nologin" \
    --no-create-home \
    --uid "${UID}" \
    "${USER}"

COPY go.mod go.mod
COPY go.sum go.sum
COPY main.go  main.go

RUN --mount=type=cache,target=/vendor go mod download
RUN --mount=type=cache,target=/root/.cache GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-w -s" -o dashboard

####### operator

FROM scratch

WORKDIR /

USER app:app

COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group
COPY --from=builder /dashboard /dashboard
COPY static /static
COPY templates /templates

ENTRYPOINT ["/dashboard"]
