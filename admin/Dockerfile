FROM golang:1.26.2 AS build

WORKDIR /src

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -trimpath -o /admin .

FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=build --chown=app:app --chmod=755 /admin /app/admin

USER app

EXPOSE 8081

ENTRYPOINT ["/app/admin"]
