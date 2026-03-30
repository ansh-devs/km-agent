FROM golang:1.25.3-alpine3.22 AS buildstage

# Accept GitHub token at build time for private repo access
ARG GITHUB_TOKEN
ENV GOPRIVATE=github.com/kloudmate/*
ENV GONOSUMDB=github.com/kloudmate/*
RUN apk add --no-cache git make
RUN git config --global url."https://${GITHUB_TOKEN}:x-oauth-basic@github.com/".insteadOf "https://github.com/"
RUN mkdir -p /build/modules && \
    git clone https://github.com/kloudmate/kloudmate-ebpf-programs.git /build/modules/km-ebpf && \
    cd /build/modules/km-ebpf && make docker-generate

RUN git clone https://github.com/kloudmate/ai-code.git /build/modules/km-classifier

WORKDIR /app
COPY go.mod go.sum ./
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -a -tags linux -ldflags "-w -s -X 'main.version=${VERSION}'" -o /kmagent ./cmd/kmagent/...

FROM gcr.io/distroless/static-debian11
COPY --from=buildstage /kmagent /kmagent
COPY ./configs/docker-col-config.yaml /config.yaml
EXPOSE 4317 4318
ENTRYPOINT ["/kmagent", "--docker-mode", "--config", "/config.yaml", "start"]
