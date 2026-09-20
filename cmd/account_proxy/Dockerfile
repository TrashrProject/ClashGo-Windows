FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd/account_proxy ./cmd/account_proxy
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/clashgo-account ./cmd/account_proxy

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/clashgo-account /clashgo-account
EXPOSE 8787
ENV CLASHGO_ACCOUNT_LISTEN=:8787
ENTRYPOINT ["/clashgo-account"]
