#!/bin/sh

export PATH=/opt:$PATH

REDIS_CONFIG="/tmp/redis.conf"
ANNOUNCE_CONFIG="/data/announce.conf"
CLUSTER_CONFIG="/data/nodes.conf"
OPERATOR_PASSWORD_FILE="/account/password"
TLS_DIR="/tls"

echo "# Run: cluster heal"
/opt/redis-tools cluster heal || exit 1

if [[ -f ${CLUSTER_CONFIG} ]]; then
    if [[ -z "${POD_IP}" ]]; then
        echo "Unable to determine Pod IP address!"
        exit 1
    fi
    echo "Updating my IP to ${POD_IP} in ${CLUSTER_CONFIG}"
    sed -i.bak -e "/myself/ s/ .*:[0-9]*@[0-9]*/ ${POD_IP}:6379@16379/" ${CLUSTER_CONFIG}
fi

cat /conf/redis.conf > ${REDIS_CONFIG}

password=$(cat ${OPERATOR_PASSWORD_FILE})

# when redis acl supported, inject acl config
if [[ ! -z "${ACL_CONFIGMAP_NAME}" ]]; then
    echo "# Run: generate acl "
    /opt/redis-tools helper generate acl --name ${ACL_CONFIGMAP_NAME} --namespace ${NAMESPACE} >> ${REDIS_CONFIG} || exit 1
fi

if [[ "${ACL_ENABLED}" = "true" ]]; then
    if [ ! -z "${OPERATOR_USERNAME}" ]; then
        echo "masteruser \"${OPERATOR_USERNAME}\"" >> ${REDIS_CONFIG}
    fi
    if [ ! -z "${password}" ]; then
        echo "masterauth \"${password}\"" >> ${REDIS_CONFIG}
    fi
elif [[ ! -z "${password}" ]]; then
    echo "masterauth \"${password}\"" >> ${REDIS_CONFIG}
    echo "requirepass \"${password}\"" >> ${REDIS_CONFIG}
fi

if [[ -f ${ANNOUNCE_CONFIG} ]]; then
    echo "append announce conf to redis config"
    cat ${ANNOUNCE_CONFIG} >> ${REDIS_CONFIG}
fi

POD_IPS_LIST=$(echo "${POD_IPS}"|sed 's/,/ /g')

ARGS="--cluster-enabled yes --cluster-config-file ${CLUSTER_CONFIG} --protected-mode no"

if [ ! -z "${POD_IPS}"  ]; then
	if [[ "${IP_FAMILY_PREFER}" = "IPv6" ]]; then
		POD_IPv6=$(echo ${POD_IPS_LIST} |grep -E '(([0-9a-fA-F]{1,4}:){7,7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:)|fe80:(:[0-9a-fA-F]{0,4}){0,4}%[0-9a-zA-Z]{1,}|::(ffff(:0{1,4}){0,1}:){0,1}((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])|([0-9a-fA-F]{1,4}:){1,4}:((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9]))' -o  -C  1)
		ARGS="${ARGS} --bind ${POD_IPv6} ::1"
	else
        ARGS="${ARGS} --bind ${POD_IPS_LIST} localhost"
	fi
fi


if [[ "${TLS_ENABLED}" = "true" ]]; then
    ARGS="${ARGS} --port 0 --tls-port 6379 --tls-cluster yes --tls-replication yes --tls-cert-file ${TLS_DIR}/tls.crt --tls-key-file ${TLS_DIR}/tls.key --tls-ca-cert-file ${TLS_DIR}/ca.crt"
fi

chmod 0640 ${REDIS_CONFIG}

redis-server ${REDIS_CONFIG} ${ARGS} $@
