FROM debian:latest

ARG TAILWIND_VERSION=4.1.12

RUN apt-get update \
    && apt-get install -y wget \
    && wget https://github.com/tailwindlabs/tailwindcss/releases/download/v${TAILWIND_VERSION}/tailwindcss-linux-x64 -O /usr/local/bin/tailwindcss \
    && chmod +x /usr/local/bin/tailwindcss

ADD tailwind.css /config/

WORKDIR /build

CMD ["tailwindcss", "-i", "/config/tailwind.css", "-o", "/output/main.css"]
