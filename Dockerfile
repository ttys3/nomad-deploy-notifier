FROM ubuntu:23.10

COPY nomad-event-notifier  /usr/local/bin/

RUN mkdir -p /etc/nomad.d/cert

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
