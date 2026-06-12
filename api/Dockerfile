# --- build ---
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server

# --- runtime (imagen mínima) ---
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/server /server
# Migraciones junto al binario: el server las aplica al arrancar.
COPY --from=build /src/db/migrations /db/migrations
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]
