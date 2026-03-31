FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=UTC

# Install only what Ubuntu ships by default on a real server
RUN apt-get update -qq && apt-get install -y -qq \
    openssh-client \
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy the pre-built davit binary
COPY davit /usr/local/bin/davit
RUN chmod +x /usr/local/bin/davit

# Copy the test script
COPY run-tests.sh /run-tests.sh
RUN chmod +x /run-tests.sh

ENTRYPOINT ["/run-tests.sh"]
