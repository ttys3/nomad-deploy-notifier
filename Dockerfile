FROM ubuntu:24.04

COPY nomad-event-notifier  /usr/local/bin/

RUN mkdir -p /etc/nomad.d/cert; \
    apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends tzdata ca-certificates; \
    update-ca-certificates -f; \
    apt-get purge -y --auto-remove -o APT::AutoRemove::RecommendsImportant=false; \
    apt autoremove -y; \
    rm -rf /var/lib/apt/lists/*


# Add Tini
ENV TINI_VERSION v0.19.0
ADD https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini /tini
RUN chmod +x /tini

WORKDIR /usr/local/bin/

ENV TZ=Asia/Shanghai \
SLACK_TOKEN="" \
SLACK_CHANNEL="" \
NOMAD_SERVER_EXTERNAL_URL="" \
NOMAD_ADDR=https://127.0.0.1:4646 \
NOMAD_CACERT="" \
NOMAD_CLIENT_CERT="" \
NOMAD_CLIENT_KEY=""

ENTRYPOINT ["/tini", "--"]
# Run your program under Tini
CMD ["/usr/local/bin/nomad-event-notifier"]
