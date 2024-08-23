#!/bin/sh

export PATH=/opt:$PATH

RAW_SENTINEL_CONFIG="/conf/sentinel.conf"
SENTINEL_CONFIG="/data/sentinel.conf"
ANNOUNCE_CONFIG="/data/announce.conf"
OPERATOR_PASSWORD_FILE="/account/password"
TLS_DIR="/tls"

cat ${RAW_SENTINEL_CONFIG} > ${SENTINEL_CONFIG}

# Append password to sentinel configuration if it exists
password=$(cat ${OPERATOR_PASSWORD_FILE} 2>/dev/null)
if [ -n "${password}" ]; then
    echo "requirepass \"${password}\"" >> ${SENTINEL_CONFIG}
fi

# Append announce configuration to sentinel configuration if it exists
if [ -f ${ANNOUNCE_CONFIG} ]; then
    echo "# append announce conf to sentinel config"
    cat "${ANNOUNCE_CONFIG}" | grep "announce" | sed "s/^/sentinel /" >> ${SENTINEL_CONFIG}
fi

# Merge custom configuration
/opt/redis-tools sentinel merge-config --local-conf-file "${SENTINEL_CONFIG}"

# Determine localhost based on IP family preference
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

if [ "${LISTEN}" != "${POD_IP}" ]; then
    LISTEN="${LISTEN} ${POD_IP}"
fi
# Construct arguments for redis-server
ARGS="--sentinel --protected-mode no --bind ${LISTEN} ${LOCALHOST}"

# Add TLS arguments if TLS is enabled
if [ "${TLS_ENABLED}" = "true" ]; then
    ARGS="${ARGS} --port 0 --tls-port 26379 --tls-replication yes --tls-cert-file ${TLS_DIR}/tls.crt --tls-key-file ${TLS_DIR}/tls.key --tls-ca-cert-file ${TLS_DIR}/ca.crt"
else
    ARGS="${ARGS} --port 26379"
fi

# Set permissions for sentinel configuration
chmod 0600 ${SENTINEL_CONFIG}

# Start redis-server with the constructed arguments
redis-server ${SENTINEL_CONFIG} ${ARGS} $@ | sed 's/auth-pass .*/auth-pass ******/'
