#!/bin/sh

chmod -f 644 /data/*.rdb /data/*.aof /data/*.conf > /dev/null || true
chown -f 999:1000 /data/*.rdb /data/*.aof /data/*.conf > /dev/null || true


if [ "$SERVICE_TYPE" = "LoadBalancer" ] || [ "$SERVICE_TYPE" = "NodePort" ] || [ -n "$IP_FAMILY_PREFER" ] ; then
    echo "check pod binded service"
    /opt/redis-tools cluster expose || exit 1
fi

# copy binaries
cp /opt/* /mnt/opt/ && chmod 555 /mnt/opt/*
