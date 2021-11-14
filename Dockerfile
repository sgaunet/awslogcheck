FROM sgaunet/mdtohtml:0.5.1 AS mdtohtml

FROM golang:1.17.3-alpine AS builder
LABEL stage=builder

RUN apk add --no-cache upx 
ENV GOPATH /go
COPY  src/ /go/src/
WORKDIR /go/src/

RUN echo $GOPATH
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build . 
RUN upx awslogcheck




FROM alpine:3.14.2 AS final
LABEL maintainer="Sylvain Gaunet <sgaunet@gmail.com>"

RUN apk add --no-cache curl bash
RUN addgroup -S logcheck_group -g 1000 && adduser -S logcheck -G logcheck_group --uid 1000

ENV SUPERCRONIC_VERSION="v0.1.11" \
    SUPERCRONIC_PACKAGE=supercronic-linux-amd64 \
    SUPERCRONIC_SHA1SUM=a2e2d47078a8dafc5949491e5ea7267cc721d67c

ENV SUPERCRONIC_URL=https://github.com/aptible/supercronic/releases/download/$SUPERCRONIC_VERSION/$SUPERCRONIC_PACKAGE

# install dependencies
RUN apk add --update --no-cache ca-certificates curl \
# install supercronic
    && curl -fsSLO "$SUPERCRONIC_URL" \
    && echo "${SUPERCRONIC_SHA1SUM}  ${SUPERCRONIC_PACKAGE}" | sha1sum -c - \
    && chmod +x "${SUPERCRONIC_PACKAGE}" \
    && mv "${SUPERCRONIC_PACKAGE}" "/bin/${SUPERCRONIC_PACKAGE}" \
    && ln -s "/bin/${SUPERCRONIC_PACKAGE}" /bin/supercronic 

WORKDIR /opt/awslogcheck
COPY --from=builder /go/src/awslogcheck .
COPY --from=mdtohtml /usr/bin/mdtohtml /usr/bin/mdtohtml
COPY "resources" /

USER logcheck

ENTRYPOINT ["/opt/awslogcheck/entrypoint.sh"]
CMD ["supercronic","-debug","/app/cron"]