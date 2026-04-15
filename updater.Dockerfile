FROM golang:1.25.8-alpine3.22 AS buildstage
ARG TARGETARCH
ARG TARGETOS=linux
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
COPY . .

ARG VERSION=dev
ARG COMMIT_SHA=unknown
RUN go mod tidy
RUN --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=0 go build -tags kubernetes -ldflags="-w -s -X 'main.version=$VERSION' -X 'main.commit=$COMMIT_SHA'" -o configupdater ./cmd/configupdater/main.go

FROM alpine:latest
COPY --from=buildstage /app/configupdater ./configupdater
ENTRYPOINT ["./configupdater", "run"]