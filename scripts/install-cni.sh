#!/bin/sh

# Script to install Anchor CNI on a Kubernetes host.
# - Expects the host CNI binary path to be mounted at /host/opt/cni/bin.
# - Expects the host CNI network config path to be mounted at /host/etc/cni/net.d.
# - Expects the desired CNI config in the CNI_NETWORK_CONFIG env variable.

# Ensure all variables are defined, and that the script fails when an error is hit.
set -u -e

# Capture the usual signals and exit from the script
trap 'echo "SIGINT received, simply exiting..."; exit 0' SIGINT
trap 'echo "SIGTERM received, simply exiting..."; exit 0' SIGTERM
trap 'echo "SIGHUP received, simply exiting..."; exit 0' SIGHUP

OCTOPUS=""
NODE_IPS=""
# Create macvlan interface
if [ "$CREATE_MACVLAN" == "true" ]; then
  OIFS=$IFS
  IFS=";"
  hostname=$(hostname)
  # Remove all spaces in the CLUSTER_NETWORK
  CLUSTER_NETWORK=${CLUSTER_NETWORK//[[:blank:]]/}
  suffix=0
  # hostname, master, ip, gateway, mask
  for t in $CLUSTER_NETWORK; do
    if [ "$hostname" == "$(echo $t | cut -d',' -f1)" ]; then
      # TODO: check invalidation of the inputs.
      master="$(echo $t | cut -d',' -f2)"
      ip="$(echo $t | cut -d',' -f3)"
      gateway="$(echo $t | cut -d',' -f4)"
      mask="$(echo $t | cut -d',' -f5)"
      a=$(echo $ip | cut -d'.' -f1)
      b=$(echo $ip | cut -d'.' -f2)
      c=$(echo $ip | cut -d'.' -f3)
      d=$(echo $ip | cut -d'.' -f4)
      ip_int="$((a * 256 ** 3 + b * 256 ** 2 + c * 256 + d))"
      subnet_int=$(($ip_int & (0xffffffff - (1<<32-$mask) + 1)))

      delim=""
      subnet=""
      # Caculate the subnet.
      for e in 3 2 1 0; do
        octet=$(($subnet_int / (256 ** $e)))
        subnet_int=$((subnet_int -= octet * 256 ** $e))
        subnet=$subnet$delim$octet
        delim=.
      done
      subnet=$subnet/$mask
      # Restore the IFS.
      IFS=";"
      NODE_IPS=$NODE_IPS\"$ip\",
      OCTOPUS=$OCTOPUS\"${subnet}\":\"${master}\",
      MACVLAN_INTERFACE=$master
      noskip=false
      if [ ${#suffix} -gt 2 ]; then
        echo "Max 100 interfaces are support" && exit 1
      fi
      if [ ${#suffix} -eq 1 ]; then
        macvlan=acr0"$suffix"
      else
        macvlan=acr"$suffix"
      fi
      ip addr | grep -oE "${macvlan}" > /dev/null 2>&1 || noskip=true
      if [ "$noskip" == "true" ]; then
        echo "Turnning $master promisc on..."
        ip link set $master promisc on
        echo "Creating macvlan interface..."
        interface_created=false
        if [ $interface_created == "false" ]; then
          # We write in this way since we have set -eu in the header of this script.
          interface_created=true
          ip link add $macvlan link $master type macvlan mode bridge > /dev/null 2>&1 || interface_created=false
          suffix=$((suffix+1))
        fi
        if [ $interface_created == "false" ]; then
          echo "Cannot create macvlan interface, will exit soon"
          ip link set dev $master up
          exit 1
        fi

        ip link set dev $master up

        echo "Deleting $ip from device $master..."
        ip addr del $ip/$mask dev $master || true

        echo "Adding $ip to device $macvlan..."
        ip addr add $ip/$mask dev $macvlan

        echo "Turnning on $macvlan and flushing the route infomation..."
        ip link set dev $macvlan up
        ip route flush dev $macvlan

        echo "Replacing the route for $subnet..."
        ip route replace $subnet dev $macvlan metric 0

        # Only change the route for default at the first interface.
        if [ $macvlan == "acr00" ]; then
          echo "Replacing the route for default..."
          ip route add default via $gateway dev $macvlan || \
          ip route replace default via $gateway dev $macvlan || true
        fi
        # Ping the gateway for fast flushing the cache on the switch.
        ping -c 4 $gateway > /dev/null 2>&1 || true
      else
        echo "MacVLAN insterface ${macvlan} for anchor exists, Check the information below: "
        echo ""
        echo "Hostname: $hostname"
        echo ""
        ip addr | grep -oE "${macvlan}@$master" | grep -oE "${macvlan}" | xargs -n 1 ip addr show

        echo ""
        echo "It may be caused by: "
        echo "    1. This pod which belongs to anchor daemonset get killed and restart"
        echo "    2. Somebody create ${macvlan} manaully"
        echo "    3. Something error in pre-installation and then re-deploy anchor"
        echo "    4. The ${macvlan} interface is remains by the old env"
        echo ""
        echo "What you can do are: "
        echo "    When 1. Nothing to do"
        echo "    When 2. Delete it manaully Or it will be deleted by node restart"
        echo "    When 3. Nothing to do if cluster network not changed, else restart the node"
        echo "    When 4. Restart the node"
        suffix=$((suffix+1))
      fi
    fi
  done
  IFS=$OIFS
fi

# The directory on the host where CNI networks are installed. Defaults to
# /etc/cni/net.d, but can be overridden by setting CNI_NET_DIR.  This is used
# for populating absolute paths in the CNI network config to assets
# which are installed in the CNI network config directory.
HOST_CNI_NET_DIR=${CNI_NET_DIR:-/etc/cni/net.d}
HOST_SECRETS_DIR=${HOST_CNI_NET_DIR}/anchor-tls

# Directory where we expect that TLS assets will be mounted into
# the anchor/cni container.
SECRETS_MOUNT_DIR=${TLS_ASSETS_DIR:-/anchor-secrets}

# Clean up any existing binaries / config / assets.
rm -f /host/opt/cni/bin/anchor-ipam
rm -f /host/etc/cni/net.d/anchor-tls/*

# Copy over any TLS assets from the SECRETS_MOUNT_DIR to the host.
# First check if the dir exists and has anything in it.
if [ "$(ls ${SECRETS_MOUNT_DIR} 3>/dev/null)" ];
then
	echo "Installing any TLS assets from ${SECRETS_MOUNT_DIR}"
	mkdir -p /host/etc/cni/net.d/anchor-tls
	cp -p ${SECRETS_MOUNT_DIR}/* /host/etc/cni/net.d/anchor-tls/
fi

# If the TLS assets actually exist, update the variables to populate into the
# CNI network config.  Otherwise, we'll just fill that in with blanks.
if [ -e "/host/etc/cni/net.d/anchor-tls/etcd-ca" ];
then
	CNI_CONF_ETCD_CA=${HOST_SECRETS_DIR}/etcd-ca
fi

if [ -e "/host/etc/cni/net.d/anchor-tls/etcd-key" ];
then
	CNI_CONF_ETCD_KEY=${HOST_SECRETS_DIR}/etcd-key
fi

if [ -e "/host/etc/cni/net.d/anchor-tls/etcd-cert" ];
then
	CNI_CONF_ETCD_CERT=${HOST_SECRETS_DIR}/etcd-cert
fi

# Choose which default cni binaries should be copied
SKIP_CNI_BINARIES=${SKIP_CNI_BINARIES:-""}
SKIP_CNI_BINARIES=",$SKIP_CNI_BINARIES,"
UPDATE_CNI_BINARIES=${UPDATE_CNI_BINARIES:-"true"}

# Place the new binaries if the directory is writeable.
for dir in /host/opt/cni/bin /host/secondary-bin-dir
do
	if [ ! -w "$dir" ];
	then
		echo "$dir is non-writeable, skipping"
		continue
	fi
	for path in /opt/cni/bin/*;
	do
		filename="$(basename $path)"
		tmp=",$filename,"
		if [ "${SKIP_CNI_BINARIES#*$tmp}" != "$SKIP_CNI_BINARIES" ];
		then
			echo "$filename is in SKIP_CNI_BINARIES, skipping"
			continue
		fi
		if [ "${UPDATE_CNI_BINARIES}" != "true" -a -f $dir/$filename ];
		then
			echo "$dir/$filename is already here and UPDATE_CNI_BINARIES isn't true, skipping"
			continue
		fi
		cp $path $dir/
		if [ "$?" != "0" ];
		then
			echo "Failed to copy $path to $dir. This may be caused by selinux configuration on the host, or something else."
			exit 1
		fi
	done

	echo "Wrote Anchor CNI binaries to $dir"
  # TODO: log version.
done

TMP_CONF='/anchor.conf.tmp'
# If specified, overwrite the network configuration file.
if [ "${CNI_NETWORK_CONFIG:-}" != "" ]; then
cat >$TMP_CONF <<EOF
${CNI_NETWORK_CONFIG:-}
EOF
fi

# Pull out service account token.
SERVICEACCOUNT_TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)

# Check if we're running as a k8s pod.
if [ -f "/var/run/secrets/kubernetes.io/serviceaccount/token" ]; then
	# We're running as a k8d pod - expect some variables.
	if [ -z ${KUBERNETES_SERVICE_HOST} ]; then
		echo "KUBERNETES_SERVICE_HOST not set"; exit 1;
	fi
	if [ -z ${KUBERNETES_SERVICE_PORT} ]; then
		echo "KUBERNETES_SERVICE_PORT not set"; exit 1;
	fi

	# Write a kubeconfig file for the CNI plugin.  Do this
	# to skip TLS verification for now.  We should eventually support
	# writing more complete kubeconfig files. This is only used
	# if the provided CNI network config references it.
	touch /host/etc/cni/net.d/anchor-kubeconfig
	chmod ${KUBECONFIG_MODE:-600} /host/etc/cni/net.d/anchor-kubeconfig
	cat > /host/etc/cni/net.d/anchor-kubeconfig <<EOF
# Kubeconfig file for Anchor CNI plugin.
apiVersion: v1
kind: Config
clusters:
- name: local
  cluster:
    server: ${KUBERNETES_SERVICE_PROTOCOL:-https}://[${KUBERNETES_SERVICE_HOST}]:${KUBERNETES_SERVICE_PORT}
    insecure-skip-tls-verify: true
users:
- name: anchor
  user:
    token: "${SERVICEACCOUNT_TOKEN}"
contexts:
- name: anchor-context
  context:
    cluster: local
    user: anchor
current-context: anchor-context
EOF

fi


# Insert any of the supported "auto" parameters.
grep "__KUBERNETES_SERVICE_HOST__" $TMP_CONF && sed -i s/__KUBERNETES_SERVICE_HOST__/${KUBERNETES_SERVICE_HOST}/g $TMP_CONF
grep "__KUBERNETES_SERVICE_PORT__" $TMP_CONF && sed -i s/__KUBERNETES_SERVICE_PORT__/${KUBERNETES_SERVICE_PORT}/g $TMP_CONF
sed -i s/__KUBERNETES_NODE_NAME__/${KUBERNETES_NODE_NAME:-$(hostname)}/g $TMP_CONF
sed -i s/__KUBECONFIG_FILENAME__/anchor-kubeconfig/g $TMP_CONF
sed -i s/__CNI_MTU__/${CNI_MTU:-1500}/g $TMP_CONF

# Use alternative command character "~", since these include a "/".
sed -i s~__KUBECONFIG_FILEPATH__~${HOST_CNI_NET_DIR}/anchor-kubeconfig~g $TMP_CONF
# TODO: make it back.
# sed -i s~__ETCD_CERT_FILE__~${CNI_CONF_ETCD_CERT:-}~g $TMP_CONF
# sed -i s~__ETCD_KEY_FILE__~${CNI_CONF_ETCD_KEY:-}~g $TMP_CONF
# sed -i s~__ETCD_CA_CERT_FILE__~${CNI_CONF_ETCD_CA:-}~g $TMP_CONF
sed -i s~__ETCD_ENDPOINTS__~${ETCD_ENDPOINTS:-}~g $TMP_CONF
sed -i s~__MACVLAN_INTERFACE__~${MACVLAN_INTERFACE:-}~g $TMP_CONF
sed -i s~__ETCD_KEY_FILE__~${ETCD_KEY:-}~g $TMP_CONF
sed -i s~__ETCD_CERT_FILE__~${ETCD_CERT:-}~g $TMP_CONF
sed -i s~__ETCD_CA_CERT_FILE__~${ETCD_CA:-}~g $TMP_CONF

sed -i s~__SERVICE_CLUSTER_IP_RANGE__~${SERVICE_CLUSTER_IP_RANGE:-}~g $TMP_CONF
sed -i s~__NODE_IPS__~${NODE_IPS%,}~g $TMP_CONF
sed -i s~__ANCHOR_MODE__~${ANCHOR_MODE:-}~g $TMP_CONF
if [ "${ANCHOR_MODE}" == "octopus" ]; then
  sed -i /\"master\":/d $TMP_CONF
  sed -i s~__OCTOPUS__~${OCTOPUS%,}~g $TMP_CONF
elif [ "${ANCHOR_MODE}" == "macvlan" ]; then
  sed -i /\"octopus\":/d $TMP_CONF
fi

# sed -i s~__LOG_LEVEL__~${LOG_LEVEL:-warn}~g $TMP_CONF
CNI_CONF_NAME=${CNI_CONF_NAME:-10-anchor.conf}
CNI_OLD_CONF_NAME=${CNI_OLD_CONF_NAME:-10-anchor.conf}

# Log the config file before inserting service account token.
# This way auth token is not visible in the logs.
echo "CNI config: $(cat ${TMP_CONF})"

sed -i s/__SERVICEACCOUNT_TOKEN__/${SERVICEACCOUNT_TOKEN:-}/g $TMP_CONF

# Delete old CNI config files for upgrades.
if [ "${CNI_CONF_NAME}" != "${CNI_OLD_CONF_NAME}" ]; then
    rm -f "/host/etc/cni/net.d/${CNI_OLD_CONF_NAME}"
fi
# Move the temporary CNI config into place.
mv $TMP_CONF /host/etc/cni/net.d/${CNI_CONF_NAME}
if [ "$?" != "0" ];
then
	echo "Failed to mv files. This may be caused by selinux configuration on the host, or something else."
	exit 1
fi

echo "Created CNI config ${CNI_CONF_NAME}"

# Unless told otherwise, sleep forever.
# This prevents Kubernetes from restarting the pod repeatedly.
should_sleep=${SLEEP:-"true"}
echo "Done configuring CNI.  Sleep=$should_sleep"
while [ "$should_sleep" == "true"  ]; do
	# Kubernetes Secrets can be updated.  If so, we need to install the updated
	# version to the host. Just check the timestamp on the certificate to see if it
	# has been updated.  A bit hokey, but likely good enough.
	if [ "$(ls ${SECRETS_MOUNT_DIR} 2>/dev/null)" ];
	then
        stat_output=$(stat -c%y ${SECRETS_MOUNT_DIR}/etcd-cert 2>/dev/null)
        sleep 10;
        if [ "$stat_output" != "$(stat -c%y ${SECRETS_MOUNT_DIR}/etcd-cert 2>/dev/null)" ]; then
            echo "Updating installed secrets at: $(date)"
            cp -p ${SECRETS_MOUNT_DIR}/* /host/etc/cni/net.d/anchor-tls/
        fi
    else
        sleep 10
    fi
done

