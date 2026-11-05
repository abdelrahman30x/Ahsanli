#!/bin/bash
# Build and deploy the AHSANLY Next.js bundle to the VPS.
# Run from the workspace root.
set -e
SERVER=${AHSANLY_SERVER:-root@134.122.89.95}
REMOTE_DIR=${AHSANLY_REMOTE_DIR:-/srv/ahsanly}

cd "$(dirname "$0")/.."
cd web

echo "Building Next.js..."
npm run build

echo "Preparing standalone bundle..."
rm -rf .next/standalone/.next/static .next/standalone/public
cp -r .next/static .next/standalone/.next/static
cp -r public .next/standalone/public

echo "Creating archive..."
tar -czf /tmp/ahsanly-web-standalone.tar.gz -C .next/standalone .

echo "Deploying to ${SERVER}..."
scp /tmp/ahsanly-web-standalone.tar.gz "${SERVER}:/tmp/ahsanly-web-standalone.tar.gz"
ssh "${SERVER}" <<EOF
set -e
systemctl stop ahsanly-web.service
rm -rf ${REMOTE_DIR}/web_next.old
mv ${REMOTE_DIR}/web_next ${REMOTE_DIR}/web_next.old
mkdir -p ${REMOTE_DIR}/web_next
tar -xzf /tmp/ahsanly-web-standalone.tar.gz -C ${REMOTE_DIR}/web_next
chown -R botdeals:botdeals ${REMOTE_DIR}/web_next
systemctl start ahsanly-web.service
sleep 2
systemctl is-active ahsanly-web.service
EOF

echo "Deployed."
