FROM golang:1.22-alpine AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum* ./backend/
RUN cd backend && go mod download
COPY backend ./backend
RUN cd backend && CGO_ENABLED=0 go build -o /out/cyverra-api ./cmd/api
FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /out/cyverra-api /cyverra-api
COPY backend/migrations /app/migrations
ENV MIGRATIONS_DIR=/app/migrations
ENTRYPOINT ["/cyverra-api"]
