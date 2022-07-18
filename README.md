# tailscale2cloudflare

A tiny utility to run periodically that updates DNS entries for your Tailscale devices in a Cloudflare zone. It's currently being used to keep a globally-distributed K3S cluster running.

It can(not):

- [x] Create A records based on Tailscale hostnames
- [ ] Update existing A records
- [x] Support subdomain suffixes
- [ ] Support multiple A records for a host

`grep -F TODO` to see the various complicated things that need to be done.


## Note on hostnames, machine names

Per https://github.com/mark-ignacio/tailscale2cloudflare/issues/2, it's possible to have a device hostname that isn't a valid DNS name. 

As of 07/18/2022, tailscale2cloudflare has switched to using [machine names](https://tailscale.com/kb/1098/machine-names/), which parallels Tailscale's MagicDNS implementation. To retain the old behavior of using hostnames, use the `--sync-hostnames` flag or set `SYNC_HOSTNAMES=1`.