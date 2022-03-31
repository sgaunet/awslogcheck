FROM scratch AS final
LABEL maintainer="Sylvain Gaunet <sgaunet@gmail.com>"
WORKDIR /opt/awslogcheck
COPY awslogcheck .
COPY "resources" /
USER awslogcheck
CMD ["/opt/awslogcheck/awslogcheck", "-c", "/opt/awslogcheck/cfg.yaml"]
