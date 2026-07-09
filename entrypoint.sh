#!/bin/bash
set -e

cd /

# If running as root, fix permissions for mounted volumes
if [ "$(id -u)" = '0' ]; then
	for dir in /models /backends /data /configuration /run/localai; do
		if [ -d "$dir" ] && [ "$(stat -c '%u' "$dir")" != "1000" ]; then
			echo "Fixing permissions for $dir..."
			chown -R localai:localai "$dir"
		fi
	done
fi

# If we have set EXTRA_BACKENDS, then we need to prepare the backends
if [ -n "$EXTRA_BACKENDS" ]; then
	echo "EXTRA_BACKENDS: $EXTRA_BACKENDS"
	# Space separated list of backends
	for backend in $EXTRA_BACKENDS; do
		echo "Preparing backend: $backend"
		make -C $backend
	done
fi

echo "CPU info:"
grep -e "model\sname" /proc/cpuinfo | head -1
grep -e "flags" /proc/cpuinfo | head -1
if grep -q -e "\savx\s" /proc/cpuinfo ; then
	echo "CPU:    AVX    found OK"
else
	echo "CPU: no AVX    found"
fi
if grep -q -e "\savx2\s" /proc/cpuinfo ; then
	echo "CPU:    AVX2   found OK"
else
	echo "CPU: no AVX2   found"
fi
if grep -q -e "\savx512" /proc/cpuinfo ; then
	echo "CPU:    AVX512 found OK"
else
	echo "CPU: no AVX512 found"
fi

if [ "$(id -u)" = '0' ]; then
	exec gosu localai ./local-ai "$@"
else
	exec ./local-ai "$@"
fi
