# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY services/ledger ./services/ledger
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/ledger ./services/ledger/cmd/ledger

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ledger /app/ledger
USER nonroot:nonroot
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD ["/app/ledger", "healthcheck"]
ENTRYPOINT ["/app/ledger"]
