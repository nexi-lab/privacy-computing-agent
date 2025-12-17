# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY tsql_ctl tsql_ctl/
WORKDIR tsql_ctl

RUN go env -w GOPROXY=https://goproxy.cn,direct
RUN go mod tidy
RUN go build -ldflags="-w -s" -o tsqlctl .

FROM ubuntu/mysql

# ---------- Mirror Sources ----------
RUN sed -i 's/archive.ubuntu.com/mirrors.tuna.tsinghua.edu.cn/g' /etc/apt/sources.list && \
    sed -i 's/security.ubuntu.com/mirrors.tuna.tsinghua.edu.cn/g' /etc/apt/sources.list

# ---------- Install Supervisor + tools ----------
RUN apt-get update && \
    apt-get install -y supervisor net-tools lsof dnsutils iputils-ping libgomp1 \
    && rm -rf /var/lib/apt/lists/*

# Supervisor directories
RUN mkdir -p /var/log/supervisor
RUN mkdir -p /etc/supervisor/conf.d

# ---------- Workdir ----------
WORKDIR /home/user

# ---------- Copy config files ----------
COPY config.yml /home/user/config/config.yml
COPY gflags.conf /home/user/config/gflags.conf
COPY party_info.json /home/user/config/party_info.json

COPY cert.crt /home/user/config/cert.crt
COPY key.key /home/user/config/key.key

COPY init.sql /docker-entrypoint-initdb.d/init.sql
COPY my.cnf /etc/mysql/my.cnf

COPY broker /home/user/broker
COPY brokerctl /home/user/brokerctl
COPY scqlengine /home/user/scqlengine
COPY --from=builder /build/tsql_ctl/tsqlctl /home/user/tsqlctl

ENV MYSQL_ALLOW_EMPTY_PASSWORD=true

COPY supervisord.conf /etc/supervisor/supervisord.conf
CMD ["/usr/bin/supervisord", "-n", "-c", "/etc/supervisor/supervisord.conf"]