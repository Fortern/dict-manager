FROM debian:trixie-slim

EXPOSE 8080/tcp
WORKDIR /work
ENV GIN_MODE=release

COPY build/dict-manager /usr/local/bin/dict-manager

RUN chmod +x /usr/local/bin/dict-manager

ENTRYPOINT ["/usr/local/bin/dict-manager"]
