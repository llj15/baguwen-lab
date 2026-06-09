ARG GO_IMAGE=docker.io/library/golang:1.25.0-alpine3.22@sha256:f18a072054848d87a8077455f0ac8a25886f2397f88bfdd222d6fafbb5bba440
ARG RUNTIME_IMAGE=docker.io/library/alpine:3.22@sha256:310c62b5e7ca5b08167e4384c68db0fd2905dd9c7493756d356e893909057601
ARG PYTHON_IMAGE=docker.io/library/python:3.12.11-slim-bookworm@sha256:519591d6871b7bc437060736b9f7456b8731f1499a57e22e6c285135ae657bf7

FROM ${GO_IMAGE} AS experiment-builder
WORKDIR /app
ENV CGO_ENABLED=0 \
    GOTOOLCHAIN=local

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY 01_basic_lock ./01_basic_lock
COPY 02_redlock ./02_redlock
COPY 03_watchdog ./03_watchdog
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w -buildid=" -o /bin/basic_lock ./01_basic_lock && \
    go build -trimpath -ldflags="-s -w -buildid=" -o /bin/redlock ./02_redlock && \
    go build -trimpath -ldflags="-s -w -buildid=" -o /bin/watchdog ./03_watchdog

FROM ${RUNTIME_IMAGE} AS experiment
ENV TZ=UTC \
    LANG=C.UTF-8 \
    LC_ALL=C.UTF-8 \
    OUTPUT_DIR=/data
RUN mkdir -p /data /app
COPY --from=experiment-builder /bin/basic_lock /bin/basic_lock
COPY --from=experiment-builder /bin/redlock /bin/redlock
COPY --from=experiment-builder /bin/watchdog /bin/watchdog
COPY run_all.sh /app/run_all.sh
RUN chmod +x /app/run_all.sh
ENTRYPOINT ["/app/run_all.sh"]

FROM ${PYTHON_IMAGE} AS analysis
WORKDIR /app
ENV MPLCONFIGDIR=/tmp/matplotlib \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    OUTPUT_DIR=/data

COPY conf/python-requirements.txt /tmp/python-requirements.txt
RUN pip install --no-cache-dir --disable-pip-version-check --root-user-action=ignore -r /tmp/python-requirements.txt
COPY analyze.py /app/analyze.py
COPY scripts/verify_results.py /app/scripts/verify_results.py
CMD ["python", "analyze.py"]
