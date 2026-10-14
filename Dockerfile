FROM golang:1.27-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X github.com/fbsobreira/gotron-mcp/internal/version.Version=${VERSION} -X github.com/fbsobreira/gotron-mcp/internal/version.Commit=${COMMIT}" -o /gotron-mcp ./cmd/gotron-mcp

FROM alpine:3.19
# The MCP Registry reads this label to confirm the image belongs to the
# server named in server.json.
LABEL io.modelcontextprotocol.server.name="io.github.fbsobreira/gotron-mcp"
RUN apk add --no-cache ca-certificates
COPY --from=builder /gotron-mcp /usr/local/bin/gotron-mcp
EXPOSE 8080
ENTRYPOINT ["gotron-mcp", "--transport", "http", "--bind", "0.0.0.0"]
