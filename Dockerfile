# syntax=docker/dockerfile:1

FROM golang:1.22-bookworm AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . ./

ARG TARGETOS=linux
ARG TARGETARCH=amd64

ENV CGO_ENABLED=0

RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/thule-api ./cmd/thule-api
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/thule-worker ./cmd/thule-worker

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /out/thule-api /thule-api
COPY --from=builder /out/thule-worker /thule-worker

USER nonroot:nonroot

ENTRYPOINT ["/thule-api"]
