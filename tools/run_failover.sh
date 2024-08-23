#!/bin/sh

export PATH=/opt:$PATH

# Read Redis credentials
REDIS_PASSWORD=$(cat /account/password 2>/dev/null)
REDIS_USERNAME=$(cat /account/username 2>/dev/null)
ACL_ARGS=""
ACL_CONFIG="/tmp/acl.conf"

# Copy base Redis configuration
CONFIG_FILE="/tmp/redis.conf"
cat /redis/redis.conf > "$CONFIG_FILE"

# Append username to Redis configuration if it exists
if [ -n "$REDIS_USERNAME" ]; then
    echo "masteruser \"$REDIS_USERNAME\"" >> "$CONFIG_FILE"
fi

# Check for new password file and update Redis password if it exists
if [ -e /tmp/newpass ]; then
    echo "## new passwd found"
    REDIS_PASSWORD=$(cat /tmp/newpass)
fi

# Append password to Redis configuration if it exists
if [ -n "$REDIS_PASSWORD" ]; then
    echo "requirepass \"$REDIS_PASSWORD\"" >> "$CONFIG_FILE"
    echo "masterauth \"$REDIS_PASSWORD\"" >> "$CONFIG_FILE"
fi

# Generate ACL configuration if ACL_CONFIGMAP_NAME is set
if [ -n "$ACL_CONFIGMAP_NAME" ]; then
    echo "## generate acl"
    /opt/redis-tools helper generate acl --name "$ACL_CONFIGMAP_NAME" > "$ACL_CONFIG" || exit 1
    ACL_ARGS="--aclfile $ACL_CONFIG"
fi

# Handle sentinel monitoring policy
if [ "$MONITOR_POLICY" = "sentinel" ]; then
    ANNOUNCE_CONFIG="/data/announce.conf"
    ANNOUNCE_IP=""
    ANNOUNCE_PORT=""
    if [ -f "$ANNOUNCE_CONFIG" ]; then
        echo "" >> "$CONFIG_FILE"
        cat "$ANNOUNCE_CONFIG" >> "$CONFIG_FILE"

        ANNOUNCE_IP=$(grep 'announce-ip' "$ANNOUNCE_CONFIG" | awk '{print $2}')
        ANNOUNCE_PORT=$(grep 'announce-port' "$ANNOUNCE_CONFIG" | awk '{print $2}')
    fi
    
    echo "## check and do failover"
    /opt/redis-tools sentinel failover --escape "${ANNOUNCE_IP}:${ANNOUNCE_PORT}" --escape "[${ANNOUNCE_IP}]:${ANNOUNCE_PORT}" --timeout 120
    # Get current master info
    addr=$(/opt/redis-tools sentinel get-master-addr --healthy)
    if [ $? -eq 0 ] && [ -n "$addr" ]; then
        # Check if the address is IPv6 or IPv4
        if echo "$addr" | grep -q ']:'; then
            master=$(echo "$addr" | sed -n 's/\(\[.*\]\):\([0-9]*\)/\1/p' | tr -d '[]')
            masterPort=$(echo "$addr" | sed -n 's/\(\[.*\]\):\([0-9]*\)/\2/p')
        else
            master=$(echo "$addr" | cut -d ':' -f 1)
            masterPort=$(echo "$addr" | cut -d ':' -f 2)
        fi

        echo "## current master: $addr"
        if [ "$master" != "127.0.0.1" ] && [ "$master" != "::1" ]; then
            if [ "$masterPort" != "6379" ] && [ "$masterPort" != "$ANNOUNCE_PORT" ]; then
                echo "## config $master $masterPort as my master"
                echo "" >> "$CONFIG_FILE"
                echo "slaveof $master $masterPort" >> "$CONFIG_FILE"
            elif [ "$masterPort" != "$ANNOUNCE_PORT" ] && [ "$master" != "$ANNOUNCE_IP" ]; then
                echo "## config $master $masterPort as my master"
                echo "" >> "$CONFIG_FILE"
                echo "slaveof $master $masterPort" >> "$CONFIG_FILE"
            fi
        fi
    fi
fi

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
# Listne only to protocol matched IP
ARGS="--protected-mode no --bind $LISTEN $LOCALHOST"

# Add TLS arguments if TLS is enabled
if [ "$TLS_ENABLED" = "true" ]; then
    ARGS="$ARGS --port 0 --tls-port 6379 --tls-replication yes --tls-cert-file $TLS_DIR/tls.crt --tls-key-file $TLS_DIR/tls.key --tls-ca-cert-file $TLS_DIR/ca.crt"
fi

# Set permissions for configuration files
chmod 0600 "$CONFIG_FILE"
chmod 0600 "$ACL_CONFIG"

# Start Redis server with the constructed arguments
redis-server "$CONFIG_FILE" $ACL_ARGS $ARGS $@
