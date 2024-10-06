FROM --platform=$BUILDPLATFORM golang:1.23 AS builder

WORKDIR /src
COPY . .

RUN go mod download

ENV CGO_ENABLED=0

ARG TARGETOS
ARG TARGETARCH
ENV GOOS=$TARGETOS GOARCH=$TARGETARCH
RUN go build -o /healthchecksio-cli

# distroless
FROM gcr.io/distroless/static-debian12

COPY --from=builder /healthchecksio-cli /healthchecksio-cli
ENTRYPOINT ["/healthchecksio-cli"]
