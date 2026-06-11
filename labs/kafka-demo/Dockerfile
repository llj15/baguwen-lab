ARG GO_IMAGE=docker.io/library/golang:1.24.5-alpine3.22@sha256:daae04ebad0c21149979cd8e9db38f565ecefd8547cf4a591240dc1972cf1399
ARG RUNTIME_IMAGE=docker.io/library/alpine:3.22@sha256:310c62b5e7ca5b08167e4384c68db0fd2905dd9c7493756d356e893909057601
ARG PYTHON_IMAGE=docker.io/library/python:3.12.11-slim-bookworm@sha256:519591d6871b7bc437060736b9f7456b8731f1499a57e22e6c285135ae657bf7

FROM ${GO_IMAGE} AS experiment-builder
WORKDIR /app
ENV CGO_ENABLED=0 \
    GOTOOLCHAIN=local \
    PATH=/usr/local/go/bin:/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY main.go ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w -buildid=" -o /experiment ./main.go

FROM ${RUNTIME_IMAGE} AS experiment
ENV TZ=UTC \
    LANG=C.UTF-8 \
    LC_ALL=C.UTF-8 \
    OUTPUT_DIR=/data \
    KAFKA_BROKERS=kafka:9092
RUN mkdir -p /data
COPY --from=experiment-builder /experiment /experiment
ENTRYPOINT ["/experiment"]

FROM ${PYTHON_IMAGE} AS analysis
WORKDIR /app
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    OUTPUT_DIR=/data

COPY analysis.py /app/analysis.py
COPY scripts/verify_results.py /app/scripts/verify_results.py
CMD ["python", "analysis.py"]
