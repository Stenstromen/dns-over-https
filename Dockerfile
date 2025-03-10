FROM golang:alpine AS build
WORKDIR /src
ADD . /src
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o doh-server/doh-server ./doh-server

FROM scratch
COPY --from=build /src/doh-server/doh-server /doh-server
EXPOSE 8053/tcp
EXPOSE 8053/udp
USER 65534:65534
ENTRYPOINT ["/doh-server"]