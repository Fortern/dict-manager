FROM debian:trixie-slim

EXPOSE 8080/tcp
WORKDIR /work

COPY build/dict-manager /usr/local/bin/dict-manager

RUN chmod +x /usr/local/bin/dict-manager

ENTRYPOINT ["/usr/local/bin/dict-manager"]
