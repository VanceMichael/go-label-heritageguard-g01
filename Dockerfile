FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/heritageguard ./cmd/server
RUN mkdir -p /out/data

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /app
COPY --from=build --chown=nonroot:nonroot /out/heritageguard /app/heritageguard
COPY --from=build --chown=nonroot:nonroot /out/data /data
USER nonroot:nonroot
EXPOSE 8080
ENV HERITAGEGUARD_ADDR=:8080 HERITAGEGUARD_DATABASE=/data/heritageguard.db
VOLUME ["/data"]
ENTRYPOINT ["/app/heritageguard"]
