FROM alpine

LABEL maintainer "Haines Chan <zhinhai@gmail.com>"
RUN apk --no-cache add iproute2

ADD anchor /opt/cni/bin/anchor
ADD octopus /opt/cni/bin/octopus
ADD install-cni.sh /install-cni.sh

ENV PATH=$PATH:/opt/cni/bin
VOLUME /opt/cni
WORKDIR /opt/cni/bin
CMD ["/opt/cni/bin/anchor"]
