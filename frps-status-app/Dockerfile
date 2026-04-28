FROM node:20-alpine AS frontend
WORKDIR /workspace/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build
# outDir: '../web' -> /workspace/web

FROM golang:1.22-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
COPY src/ src/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/frps-status .

FROM alpine:3.20
WORKDIR /app
COPY --from=go-build /out/frps-status /app/frps-status
COPY --from=frontend /workspace/web /app/web
EXPOSE 8080
ENTRYPOINT ["/app/frps-status"]
