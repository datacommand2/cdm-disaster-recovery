FROM golang:1.14 as builder

WORKDIR /build
COPY . .

RUN make clean build

FROM ceph:v14.2.0

ARG VERSION

COPY --from=builder /build/mirrord /bin/

ENV CDM_SERVICE_VERSION=${VERSION:-unknown}
ENV CDM_SOLUTION_NAME="CDM-Disaster-Recovery"
ENV CDM_SERVICE_DESCRIPTION="Volume Mirror Daemon"
ENV GODEBUG="madvdontneed=1"

ENTRYPOINT ["/bin/mirrord"]
