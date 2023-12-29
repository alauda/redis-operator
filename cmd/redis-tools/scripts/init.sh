#!/bin/sh

chmod -f 644 /data/*.rdb /data/*.aof /data/*.conf || true
chown -f 999:1000 /data/*.rdb /data/*.aof /data/*.conf || true


if [[ "$NODEPORT_ENABLED" = "true" ]] || [[ ! -z "$IP_FAMILY_PREFER" ]] ; then
    echo "nodeport expose"
    /opt/redis-tools cluster expose || exit 1
fi

# copy binaries
cp /opt/* /mnt/opt/ && chmod 555 /mnt/opt/*
