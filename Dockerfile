FROM golang:1.22-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN go build -o /out/baixin-switch ./cmd/baixin-switch

FROM alpine:3.20

RUN adduser -D -H baixin
USER baixin

COPY --from=build /out/baixin-switch /usr/local/bin/baixin-switch
EXPOSE 11435

ENTRYPOINT ["/usr/local/bin/baixin-switch"]
