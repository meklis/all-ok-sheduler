FROM ubuntu:18.04
LABEL maintainer="Max Boyar <max.boyar.a@gmail.com>"
RUN apt update && apt -y upgrade
ADD https://github.com/meklis/all-ok-sheduler/releases/download/1.0.1/all-ok-shedule-linux-amd64 /opt/all-ok-shedule
COPY shedule.conf.yml /opt/shedule.conf.yml
RUN chmod +x /opt/all-ok-shedule
ENTRYPOINT ["/opt/all-ok-shedule", "-c", "/opt/shedule.conf.yml"]