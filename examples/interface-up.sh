#!/bin/sh
# Example for an interface-up hook. Runs the exact same one-shot path as timer.
exec /usr/local/bin/dyndns-updater --config /etc/dyndns-updater.yaml
