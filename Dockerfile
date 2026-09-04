# The fleet server, built from source and shipped without one.
#
# Two stages: a builder that has the Go toolchain, and a runtime image that has
# nothing. The result carries the static binary, a CA bundle for whatever sits
# in front of it, and no shell — there is nothing in the image to run if
# someone gets a command into it.

FROM golang:1.24-alpine AS build

WORKDIR /src

# Dependencies first, so a source-only change does not re-download them.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# CGO_ENABLED=0 is what makes the binary static, which is what lets the
# runtime stage be scratch.
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" \
      -o /out/supportone-server ./cmd/supportone-server

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/supportone-server /supportone-server

# 65532 is the conventional non-root UID in distroless images. The data
# directory is a volume, and compose creates it owned by this user.
USER 65532:65532

EXPOSE 8080
VOLUME ["/var/lib/supportone"]

ENTRYPOINT ["/supportone-server"]
CMD ["--addr", ":8080", "--data", "/var/lib/supportone"]
