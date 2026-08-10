# syntax=docker/dockerfile:1

FROM node:22-alpine AS webbuild
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS gobuild
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=webbuild /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /out/cpa-model-panel ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata \
  && adduser -D -H -u 10001 panel
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=gobuild /out/cpa-model-panel /app/cpa-model-panel
USER panel
EXPOSE 5006
ENV LISTEN=:5006 DATA_DIR=/data
VOLUME ["/data"]
ENTRYPOINT ["/app/cpa-model-panel"]
