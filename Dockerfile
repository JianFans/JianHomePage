FROM golang:1.25.1-alpine AS build

WORKDIR /src
COPY apps/server/go.mod apps/server/go.sum ./
RUN go mod download

COPY apps/server/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/yujian-server ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build --chown=nonroot:nonroot /out/yujian-server /usr/local/bin/yujian-server

ENV HTTP_ADDRESS=0.0.0.0:8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/yujian-server"]
