FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/crawler ./cmd/crawler

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/crawler ./crawler
COPY configs/config.yaml ./configs/config.yaml
VOLUME ["/app/data"]
EXPOSE 9090
ENTRYPOINT ["./crawler"]
CMD ["--config", "configs/config.yaml"]
