FROM golang:1.25.6 AS build

WORKDIR /src

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="all=-N -l" -trimpath -o /server .

FROM alpine:3.18

RUN addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=build --chown=app:app --chmod=755 /server /app/server
COPY --from=build /src/config.toml /app/config.toml

USER app

EXPOSE 8080

ENTRYPOINT ["/app/server"]
