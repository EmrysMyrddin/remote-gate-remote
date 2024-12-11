FROM golang:1.22-bullseye AS builder

ENV NODE_VERSION=22.6.0

RUN apt update && apt install xz-utils
RUN ARCH= && dpkgArch="$(dpkg --print-architecture)" \
  && case "${dpkgArch##*-}" in \
  amd64) ARCH='x64';; \
  ppc64el) ARCH='ppc64le';; \
  s390x) ARCH='s390x';; \
  arm64) ARCH='arm64';; \
  armhf) ARCH='armv7l';; \
  i386) ARCH='x86';; \
  *) echo "unsupported architecture"; exit 1 ;; \
  esac \
  && curl -fsSLO --compressed -o "node.tar.xz" "https://nodejs.org/dist/v$NODE_VERSION/node-v$NODE_VERSION-linux-$ARCH.tar.xz" \
  && tar -xJf "./node-v$NODE_VERSION-linux-$ARCH.tar.xz" --strip-components=1 --no-same-owner
RUN ln -s /usr/local/bin/node /usr/local/bin/nodejs
RUN corepack enable

WORKDIR /usr/src/app

COPY package.json yarn.lock .yarnrc.yml ./
RUN yarn workspaces focus --production

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN CGO_ENABLED=0 go build -tags="viper_bind_struct" -o /usr/src/app/build/ -a -ldflags '-extldflags "-static"' ./...

FROM scratch
COPY --from=builder /usr/src/app/build/cmd /app
COPY --from=builder /usr/src/app/static /static
COPY --from=builder /usr/src/app/static/js/htmx.min.js /static/js/htmx.min.js
COPY --from=builder /etc/ssl/certs/ /etc/ssl/certs/

EXPOSE 80
CMD ["/app"]