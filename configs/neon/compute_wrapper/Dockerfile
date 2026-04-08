ARG REPOSITORY=ghcr.io/neondatabase
ARG COMPUTE_IMAGE=compute-node-v16
ARG TAG=latest

FROM ${REPOSITORY}/${COMPUTE_IMAGE}:${TAG}

USER root

RUN echo 'Acquire::Retries "5";' > /etc/apt/apt.conf.d/80-retries && \
    apt-get update && \
    apt-get install -y curl jq netcat-openbsd

RUN mkdir -p /ext-src/pg_hint_plan-src /postgres/contrib/file_fdw /ext-src/postgis-src/ && \
    chown postgres /ext-src/pg_hint_plan-src /postgres/contrib/file_fdw /ext-src/postgis-src

USER postgres
