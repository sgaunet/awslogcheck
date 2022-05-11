FROM busybox as build-env

FROM scratch AS final
LABEL maintainer="Sylvain Gaunet <sgaunet@gmail.com>"
WORKDIR /opt/awslogcheck
COPY awslogcheck .
COPY "resources" /
COPY --from=build-env --chown=1000:1000 /tmp /tmp
USER awslogcheck
CMD ["/opt/awslogcheck/awslogcheck", "-c", "/opt/awslogcheck/cfg.yaml"]
