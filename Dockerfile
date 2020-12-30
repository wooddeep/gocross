FROM ubuntu:14.04
MAINTAINER lihan@migu.cn

ENV DOMAIN localhost

ADD gocross /migu/gocross

CMD /migu/gocross -s -x 9090 -t 5000 -d $DOMAIN

EXPOSE 9090
EXPOSE 5000
