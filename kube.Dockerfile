FROM golang:alpine AS buildstage
ARG TARGETARCH
ARG TARGETOS=linux
ARG GITHUB_TOKEN
ENV GOPRIVATE=github.com/kloudmate/*
ENV GONOSUMDB=github.com/kloudmate/*

RUN apk add --no-cache git make
RUN git config --global url."https://${GITHUB_TOKEN}:x-oauth-basic@github.com/".insteadOf "https://github.com/"

# Clone private modules into /build/modules
RUN mkdir -p /build/modules && \
    git clone https://github.com/kloudmate/kloudmate-ebpf-programs.git /build/modules/km-ebpf && \
    cd /build/modules/km-ebpf && make docker-generate

RUN git clone https://github.com/kloudmate/ai-code.git /build/modules/km-classifier

WORKDIR /app
COPY go.mod go.sum ./
COPY . .
ARG VERSION=dev
ARG COMMIT_SHA=unknown
RUN --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=0 go build -tags kubernetes -ldflags="-w -s -X 'main.version=$VERSION' -X 'main.commit=$COMMIT_SHA'" -o kmagent ./cmd/kubeagent/main.go

FROM alpine:latest
COPY --from=buildstage /app/kmagent ./kmagent
EXPOSE 4317 4318
RUN chmod +x kmagent
ENTRYPOINT ["./kmagent"]
