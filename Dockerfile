FROM scratch

WORKDIR /work

COPY --chmod=0755 build/dict-manager /usr/local/bin/dict-manager

EXPOSE 8080/tcp
ENTRYPOINT ["/usr/local/bin/dict-manager"]
