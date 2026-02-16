FROM ghcr.io/ekaya-inc/ekaya-engine:latest

USER root
RUN mkdir -p /app/certs
COPY config.yaml /app/config.yaml
COPY certs/ /app/certs/
RUN chown -R ekaya:ekaya /app

USER ekaya
WORKDIR /app
