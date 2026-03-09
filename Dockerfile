FROM node:20-alpine AS ui-builder

WORKDIR /src/ui
COPY ui/package.json ui/pnpm-lock.yaml ./
RUN npm install --legacy-peer-deps
COPY ui/ .
RUN npm run build

FROM node:20-alpine AS demo-builder

WORKDIR /src/examples/kanban
COPY examples/kanban/package*.json ./
RUN npm ci
COPY examples/kanban/ .
RUN VITE_AYB_URL="" npx vite build

WORKDIR /src/examples/live-polls
COPY examples/live-polls/package*.json ./
RUN npm ci
COPY examples/live-polls/ .
RUN VITE_AYB_URL="" npx vite build

FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui-builder /src/ui/dist ./ui/dist
COPY --from=demo-builder /src/examples/kanban/dist ./examples/kanban/dist
COPY --from=demo-builder /src/examples/live-polls/dist ./examples/live-polls/dist

RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /ayb ./cmd/ayb

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /ayb /usr/local/bin/ayb

EXPOSE 8090

ENTRYPOINT ["ayb"]
CMD ["start"]
