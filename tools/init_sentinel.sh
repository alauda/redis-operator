#!/bin/sh

if [ "$SERVICE_TYPE" = "LoadBalancer" ] || [ "$SERVICE_TYPE" = "NodePort" ] || [ -n "$IP_FAMILY_PREFER" ] ; then
    echo "check pod binded service"
    /opt/redis-tools sentinel expose || exit 1
fi

# copy binaries
cp -r /opt/* /mnt/opt/ && chmod 555 /mnt/opt/*
