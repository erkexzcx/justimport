FROM golang:1.27-alpine AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags="-X main.version=${VERSION} -w -s" -trimpath -o /justimport ./cmd/justimport

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /justimport /justimport
ENTRYPOINT ["/justimport"]
