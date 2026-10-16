#!/bin/sh
# The host copy remains private to its owner. Seaweed's entrypoint drops to
# uid/gid1000, so deliver a separate private copy into its dedicated volume.
set -eu
umask 077
test -s /source/s3.json
temporary="/runtime/.s3.json.$$"
trap 'rm -f "$temporary"' EXIT HUP INT TERM
cp /source/s3.json "$temporary"
chmod 600 "$temporary"
chown 1000:1000 "$temporary"
mv -f "$temporary" /runtime/s3.json
chown 1000:1000 /runtime
chmod 700 /runtime
echo 'Private S3 runtime configuration prepared for uid1000'
