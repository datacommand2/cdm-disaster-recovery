FROM registry.datacommand.co.kr/golang:1.14 as builder

WORKDIR /build
COPY . .

RUN make clean build

FROM alpine

RUN apk --no-cache add tzdata

ARG VERSION

COPY --from=builder /build/migratord /bin/

ENV CDM_SERVICE_VERSION=${VERSION:-unknown}
ENV CDM_SOLUTION_NAME="CDM-Disaster-Recovery"
ENV CDM_SERVICE_DESCRIPTION="Job Migrator Daemon"

ENTRYPOINT ["/bin/migratord"]
