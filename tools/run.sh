#!/bin/sh

export PATH=/opt:$PATH

REDIS_CONFIG="/tmp/redis.conf"
ACL_CONFIG="/tmp/acl.conf"
ANNOUNCE_CONFIG="/data/announce.conf"
CLUSTER_CONFIG="/data/nodes.conf"
OPERATOR_PASSWORD_FILE="/account/password"
TLS_DIR="/tls"
ACL_ARGS=""

echo "# Run: cluster heal"
/opt/redis-tools cluster heal || exit 1

LISTEN=${POD_IP}
LOCALHOST="127.0.0.1"
if echo "${POD_IP}" | grep -q ':'; then
    LOCALHOST="::1"
fi

if echo "${POD_IPS}" | grep -q ','; then
    POD_IPS_LIST=$(echo "${POD_IPS}" | tr ',' ' ')
    for ip in $POD_IPS_LIST; do
        if [ "$IP_FAMILY_PREFER" = "IPv6" ]; then
            if echo "$ip" | grep -q ':'; then
                LISTEN="$ip"
                LOCALHOST="::1"
                break
            fi
        elif [ "$IP_FAMILY_PREFER" = "IPv4" ]; then
            if echo "$ip" | grep -q '\.'; then
                LISTEN="$ip"
                LOCALHOST="127.0.0.1"
                break
            fi
        fi
    done
fi

if [ -f ${CLUSTER_CONFIG} ]; then
    if [ -z "${LISTEN}" ]; then
        echo "Unable to determine Pod IP address!"
        exit 1
    fi
    sed -i.bak -e "/myself/ s/ .*:[0-9]*@[0-9]*/ ${LISTEN}:6379@16379/" ${CLUSTER_CONFIG}
fi

cat /conf/redis.conf > ${REDIS_CONFIG}

password=$(cat ${OPERATOR_PASSWORD_FILE} 2>/dev/null)

# when redis acl supported, inject acl config
if [ -n "${ACL_CONFIGMAP_NAME}" ]; then
    echo "# Run: generate acl"
    /opt/redis-tools helper generate acl --name ${ACL_CONFIGMAP_NAME} --namespace ${NAMESPACE} > ${ACL_CONFIG} || exit 1
    ACL_ARGS="--aclfile ${ACL_CONFIG}"
fi

if [ "${ACL_ENABLED}" = "true" ]; then
    if [ -n "${OPERATOR_USERNAME}" ]; then
        echo "masteruser \"${OPERATOR_USERNAME}\"" >> ${REDIS_CONFIG}
    fi
    if [ -n "${password}" ]; then
        echo "masterauth \"${password}\"" >> ${REDIS_CONFIG}
    fi
elif [ -n "${password}" ]; then
    echo "masterauth \"${password}\"" >> ${REDIS_CONFIG}
    echo "requirepass \"${password}\"" >> ${REDIS_CONFIG}
fi

if [ -f ${ANNOUNCE_CONFIG} ]; then
    echo "append announce conf to redis config"
    cat ${ANNOUNCE_CONFIG} >> ${REDIS_CONFIG}
fi

if [ "${LISTEN}" != "${POD_IP}" ]; then
    LISTEN="${LISTEN} ${POD_IP}"
fi
ARGS="--cluster-enabled yes --cluster-config-file ${CLUSTER_CONFIG} --protected-mode no --bind ${LISTEN} ${LOCALHOST}"

if [ "${TLS_ENABLED}" = "true" ]; then
    ARGS="${ARGS} --port 0 --tls-port 6379 --tls-cluster yes --tls-replication yes --tls-cert-file ${TLS_DIR}/tls.crt --tls-key-file ${TLS_DIR}/tls.key --tls-ca-cert-file ${TLS_DIR}/ca.crt"
fi

chmod 0600 ${REDIS_CONFIG}
chmod 0600 ${ACL_CONFIG}

redis-server ${REDIS_CONFIG} ${ACL_ARGS} ${ARGS} $@
