FROM golang:1.25 AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 go build \
    -ldflags "-X github.com/tiwillia/sankey-scorecard/cmd.Version=${VERSION} \
              -X github.com/tiwillia/sankey-scorecard/cmd.Commit=${COMMIT} \
              -X github.com/tiwillia/sankey-scorecard/cmd.BuildDate=${BUILD_DATE}" \
    -o sankey-scorecard ./cmd/sankey-scorecard

FROM registry.access.redhat.com/ubi10/ubi-minimal:latest

COPY --from=builder /build/sankey-scorecard /usr/local/bin/sankey-scorecard

EXPOSE 8080

USER 1001

ENTRYPOINT ["sankey-scorecard"]
CMD ["serve"]
