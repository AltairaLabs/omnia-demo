#!/usr/bin/env bash
# Host names shared by the deploy/azure scripts. Source it; do not run it.

# poppy_dns_names: print the Poppy demo hosts, one per line, as names relative
# to dns.zone (so "larkmere" is larkmere.<zone>). The Company hosts, their
# sign-in issuer hosts, and the shared assistant. This is the one list: the
# DNS records (poppy.sh) and anything that needs each host's full name read it
# from here, so they cannot drift apart.
poppy_dns_names() {
  printf '%s\n' larkmere wrenmoor auth.larkmere auth.wrenmoor assistant
}
