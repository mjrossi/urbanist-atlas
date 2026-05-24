# Hardened runtime for scripts/verify-org-urls.sh.
#
# Alpine gives us bash + curl in ~10 MB; busybox supplies awk/grep/sed/sort.
# ca-certificates is needed for HTTPS verification of the third-party URLs
# the script probes.
#
# Designed to be run with the lock-down flags documented in the script
# header — `--read-only --cap-drop=ALL --security-opt no-new-privileges`
# plus a tmpfs at /tmp for curl's body buffers and a writable bind mount
# at /work/tmp for the report.
FROM alpine:3.20

RUN apk add --no-cache bash curl ca-certificates

WORKDIR /work
COPY scripts/verify-org-urls.sh /usr/local/bin/verify-org-urls.sh
RUN chmod +x /usr/local/bin/verify-org-urls.sh

ENTRYPOINT ["/usr/local/bin/verify-org-urls.sh"]
