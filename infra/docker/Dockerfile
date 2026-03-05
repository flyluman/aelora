FROM golang:alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/aelora-server ./cmd/server

FROM alpine AS runtime-server
WORKDIR /app
COPY --from=build /out/aelora-server /usr/local/bin/aelora-server
COPY migrations /app/migrations
COPY frontend /app/frontend

ENTRYPOINT ["/usr/local/bin/aelora-server"]
CMD ["serve"]
